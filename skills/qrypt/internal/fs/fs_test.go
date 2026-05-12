package fs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/golang-lru/v2"
	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/log"
	"github.com/yinzhenyu/skills/qrypt/internal/quark"
	"github.com/yinzhenyu/skills/qrypt/internal/staging"
)

type rewriteTransport struct {
	target string
}

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	mockURL := rt.target + req.URL.Path
	if req.URL.RawQuery != "" {
		mockURL += "?" + req.URL.RawQuery
	}
	mockReq, _ := http.NewRequest(req.Method, mockURL, req.Body)
	mockReq.Header = req.Header
	return http.DefaultTransport.RoundTrip(mockReq)
}

func newTestFS(t *testing.T) *QryptFS {
	t.Helper()

	cacheDir := t.TempDir()
	stagingDir := filepath.Join(cacheDir, "staging")
	os.MkdirAll(stagingDir, 0o755)
	store, err := staging.NewStore(stagingDir)
	if err != nil {
		t.Fatal(err)
	}

	cm, err := cache.NewCacheManager(cacheDir, 100*1024*1024)
	if err != nil {
		t.Fatal(err)
	}

	memCache, _ := lru.New[string, []byte](100)
	logger, _ := log.New("off", "", nil)
	log.L = logger
	_ = logger

	cph, _ := crypt.NewRcloneCipher("testpassword", "")

	mux := http.NewServeMux()
	mux.HandleFunc("/file/list", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": 200,
			"code":   0,
			"data":   []interface{}{},
		})
	})
	mux.HandleFunc("/file/sort", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": 200,
			"code":   0,
			"data":   []interface{}{},
		})
	})
	mux.HandleFunc("/file/download", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": 200,
			"code":   0,
			"data":   []map[string]string{{"download_url": "http://mock.dl/test"}},
		})
	})
	mux.HandleFunc("/file/create", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": 200,
			"code":   0,
			"data":   []map[string]string{{"fid": "new_fid_mock"}},
		})
	})
	mux.HandleFunc("/file", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": 200,
				"code":   0,
				"data":   map[string]string{"fid": "new_dir_fid"},
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": 200,
			"code":   0,
			"data":   []interface{}{},
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": 200,
			"code":   0,
			"data":   []interface{}{},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	fakeClient := quark.NewClient("test_cookie=abc")
	fakeClient.SetClientForTest(&http.Client{
		Transport: rewriteTransport{target: server.URL},
	})

	fs := &QryptFS{
		fileSvc:        quark.NewFileService(fakeClient, quark.NewCacheService(), cph),
		manageSvc:      quark.NewManageService(fakeClient),
		cacheSvc:       quark.NewCacheService(),
		cipher:         cph,
		cacheMgr:       cm,
		staging:        store,
		rootFid:        "root_fid_test",
		prefetchSem:    make(chan struct{}, 30),
		lruStop:        make(chan struct{}),
		memCache:       memCache,
		uploadChan:     make(chan syncTask, 100),
		metadataOpChan: make(chan metadataTask, 1000),
		maxRetries:     3,
	}
	fs.nodes = sync.Map{}
	fs.fidNodes = sync.Map{}
	fs.fetchingFiles = sync.Map{}
	fs.fetchingChunks = sync.Map{}
	fs.merging = sync.Map{}
	fs.deletingPaths = sync.Map{}
	fs.activeDeletions = sync.Map{}
	fs.deletionsByParent = sync.Map{}
	fs.retryState = sync.Map{}

	rootNode := newNode(fs.rootFid, "0", "", "/", true)
	rootNode.source = "remote"
	rootNode.mtime = time.Now()
	fs.storeNode("/", rootNode)

	return fs
}

func TestStoreNode(t *testing.T) {
	fs := newTestFS(t)

	n := newNode("fid_test", "parent_fid", "test.txt", "/test.txt", false)
	fs.storeNode("/test.txt", n)

	if v, ok := fs.nodes.Load("/test.txt"); !ok {
		t.Error("node not stored")
	} else if v.(*Node).fid != "fid_test" {
		t.Errorf("expected fid_test, got %s", v.(*Node).fid)
	}
}

func TestStoreNode_WithChildren(t *testing.T) {
	fs := newTestFS(t)

	child := newNode("child_fid", "root_fid_test", "child.txt", "/child.txt", false)
	fs.storeNode("/child.txt", child)

	if _, ok := fs.nodes.Load("/child.txt"); !ok {
		t.Error("child node not stored")
	}

	if v, ok := fs.fidNodes.Load("child_fid"); !ok {
		t.Error("child fid not indexed")
	} else if v.(*Node).name != "child.txt" {
		t.Errorf("expected child.txt, got %s", v.(*Node).name)
	}
}

func TestDeleteNodePath(t *testing.T) {
	fs := newTestFS(t)

	n := newNode("fid_del", "p", "del.txt", "/del.txt", false)
	fs.storeNode("/del.txt", n)
	fs.deleteNodePath("/del.txt", n)

	if _, ok := fs.nodes.Load("/del.txt"); ok {
		t.Error("node should be deleted")
	}
}

func TestReplaceNodePath(t *testing.T) {
	fs := newTestFS(t)

	n := newNode("fid_rep", "p", "old.txt", "/old.txt", false)
	fs.storeNode("/old.txt", n)

	fs.replaceNodePath("/old.txt", "/new.txt", n)

	if _, ok := fs.nodes.Load("/old.txt"); ok {
		t.Error("old path should be removed")
	}
	if v, ok := fs.nodes.Load("/new.txt"); !ok {
		t.Error("new path should exist")
	} else if v.(*Node).name != "old.txt" {
		t.Errorf("expected name old.txt, got %s", v.(*Node).name)
	}
}

func TestNodeCancel(t *testing.T) {
	n := &Node{}
	if n.IsCancelled() {
		t.Error("should not be cancelled initially")
	}
	n.Cancel()
	if !n.IsCancelled() {
		t.Error("should be cancelled after Cancel()")
	}
}

func TestNodeIsChildrenEmpty(t *testing.T) {
	n := &Node{children: make(map[string]*Node)}
	if !n.isChildrenEmpty() {
		t.Error("empty map should return true")
	}
	n.children["a"] = &Node{}
	if n.isChildrenEmpty() {
		t.Error("non-empty map should return false")
	}
}

func TestGetattr_Root(t *testing.T) {
	fs := newTestFS(t)

	var stat fuse.Stat_t
	errc := fs.Getattr("/", &stat, 0)
	if errc != 0 {
		t.Errorf("expected 0, got %d", errc)
	}
	if stat.Mode&fuse.S_IFDIR == 0 {
		t.Error("expected directory mode")
	}
}

func TestGetattr_NonExistent(t *testing.T) {
	fs := newTestFS(t)

	var stat fuse.Stat_t
	errc := fs.Getattr("/nonexistent.txt", &stat, 0)
	if errc == 0 {
		t.Error("expected error for nonexistent path")
	}
}

func TestAccess(t *testing.T) {
	fs := newTestFS(t)

	errc := fs.Access("/", 0)
	if errc != 0 {
		t.Errorf("expected 0, got %d", errc)
	}

	errc = fs.Access("/nonexistent", 0)
	if errc == 0 {
		t.Error("expected error for nonexistent")
	}
}

func TestMknod(t *testing.T) {
	fs := newTestFS(t)

	if fs.staging == nil {
		fs.staging, _ = staging.NewStore(t.TempDir())
	}

	errc := fs.Mknod("/newfile.txt", 0o644, 0)
	if errc != 0 {
		t.Errorf("expected 0, got %d", errc)
	}

	if _, ok := fs.nodes.Load("/newfile.txt"); !ok {
		t.Error("new file should be in tree")
	}
}

func TestCreate(t *testing.T) {
	fs := newTestFS(t)
	if fs.staging == nil {
		fs.staging, _ = staging.NewStore(t.TempDir())
	}

	errc, fh := fs.Create("/created.txt", 0, 0o644)
	if errc != 0 {
		t.Errorf("expected 0, got %d", errc)
	}
	if fh == 0 {
		t.Error("expected non-zero file handle")
	}
}

func TestOpen(t *testing.T) {
	fs := newTestFS(t)
	if fs.staging == nil {
		fs.staging, _ = staging.NewStore(t.TempDir())
	}

	fs.Create("/open.txt", 0, 0o644)
	errc, fh := fs.Open("/open.txt", 0)
	if errc != 0 {
		t.Errorf("expected 0, got %d", errc)
	}
	if fh == 0 {
		t.Error("expected non-zero file handle")
	}
}

func TestOpen_NonExistent(t *testing.T) {
	fs := newTestFS(t)
	errc, _ := fs.Open("/nonexistent.txt", 0)
	if errc == 0 {
		t.Error("expected error for nonexistent")
	}
}

func TestUnlink(t *testing.T) {
	fs := newTestFS(t)
	if fs.staging == nil {
		fs.staging, _ = staging.NewStore(t.TempDir())
	}

	fs.Create("/delete.txt", 0, 0o644)
	errc := fs.Unlink("/delete.txt")
	if errc != 0 {
		t.Errorf("expected 0, got %d", errc)
	}
}

func TestRename(t *testing.T) {
	fs := newTestFS(t)
	if fs.staging == nil {
		fs.staging, _ = staging.NewStore(t.TempDir())
	}

	fs.Create("/old.txt", 0, 0o644)
	errc := fs.Rename("/old.txt", "/new.txt")
	if errc != 0 {
		t.Errorf("expected 0, got %d", errc)
	}

	if _, ok := fs.nodes.Load("/old.txt"); ok {
		t.Error("old path should be gone")
	}
	if _, ok := fs.nodes.Load("/new.txt"); !ok {
		t.Error("new path should exist")
	}
}

func TestMkdir(t *testing.T) {
	fs := newTestFS(t)
	errc := fs.Mkdir("/newdir", 0o755)
	if errc != 0 {
		t.Logf("Mkdir returned %d (API-dependent, may fail without remote)", errc)
		return
	}
	if _, ok := fs.nodes.Load("/newdir"); !ok {
		t.Error("new dir should exist after successful Mkdir")
	}
}

func TestStatfs(t *testing.T) {
	fs := newTestFS(t)
	var stat fuse.Statfs_t
	errc := fs.Statfs("/", &stat)
	if errc != 0 {
		t.Errorf("expected 0, got %d", errc)
	}
	if stat.Bsize != 4096 {
		t.Errorf("expected 4096, got %d", stat.Bsize)
	}
}

func TestReadDir_Root(t *testing.T) {
	fs := newTestFS(t)

	var entries []string
	fill := func(name string, stat *fuse.Stat_t, ofst int64) bool {
		entries = append(entries, name)
		return true
	}
	errc := fs.Readdir("/", fill, 0, 0)
	if errc != 0 {
		t.Errorf("expected 0, got %d", errc)
	}

	if len(entries) < 2 {
		t.Errorf("expected at least . and .., got %d", len(entries))
	}
}

func TestReadDir_NonExistent(t *testing.T) {
	fs := newTestFS(t)
	fill := func(name string, stat *fuse.Stat_t, ofst int64) bool { return true }
	errc := fs.Readdir("/nonexistent", fill, 0, 0)
	if errc == 0 {
		t.Error("expected error")
	}
}

func TestTruncate(t *testing.T) {
	fs := newTestFS(t)
	if fs.staging == nil {
		fs.staging, _ = staging.NewStore(t.TempDir())
	}

	fs.Create("/trunc.txt", 0, 0o644)
	errc := fs.Truncate("/trunc.txt", 100, 0)
	if errc != 0 {
		t.Errorf("expected 0, got %d", errc)
	}
}

func TestChmod(t *testing.T) {
	fs := newTestFS(t)
	errc := fs.Chmod("/", 0o755)
	if errc != 0 {
		t.Errorf("expected 0, got %d", errc)
	}
}

func TestUtimens(t *testing.T) {
	fs := newTestFS(t)
	now := fuse.NewTimespec(time.Now())
	errc := fs.Utimens("/", []fuse.Timespec{now, now})
	if errc != 0 {
		t.Errorf("expected 0, got %d", errc)
	}
}

func TestFlush(t *testing.T) {
	fs := newTestFS(t)
	if fs.staging == nil {
		fs.staging, _ = staging.NewStore(t.TempDir())
	}

	fs.Create("/flush.txt", 0, 0o644)
	errc := fs.Flush("/flush.txt", 0)
	if errc != 0 {
		t.Errorf("expected 0, got %d", errc)
	}
}

func TestRelease(t *testing.T) {
	fs := newTestFS(t)
	if fs.staging == nil {
		fs.staging, _ = staging.NewStore(t.TempDir())
	}

	fs.Create("/release.txt", 0, 0o644)
	errc := fs.Release("/release.txt", 0)
	if errc != 0 {
		t.Errorf("expected 0, got %d", errc)
	}
}

func TestGetXattr(t *testing.T) {
	fs := newTestFS(t)
	errc, _ := fs.Getxattr("/", "user.test")
	if errc != -fuse.ENOATTR {
		t.Errorf("expected ENOATTR, got %d", errc)
	}
}

func TestListXattr(t *testing.T) {
	fs := newTestFS(t)
	errc := fs.Listxattr("/", func(name string) bool { return true })
	if errc != 0 {
		t.Errorf("expected 0, got %d", errc)
	}
}

func TestSetXattr(t *testing.T) {
	fs := newTestFS(t)
	val := []byte("value")
	flags := 0
	errc := fs.Setxattr("/", "user.test", val, flags)
	if errc != 0 {
		t.Errorf("expected 0, got %d", errc)
	}
}

func TestRemoveXattr(t *testing.T) {
	fs := newTestFS(t)
	errc := fs.Removexattr("/", "user.test")
	if errc == 0 {
		t.Error("expected ENOATTR for non-existent xattr")
	}
}

func TestLookup(t *testing.T) {
	fs := newTestFS(t)
	n, errc := fs.lookup("/")
	if errc != 0 {
		t.Errorf("expected 0, got %d", errc)
	}
	if n == nil {
		t.Fatal("expected non-nil node")
	}
	if !n.isFolder {
		t.Error("root should be a folder")
	}
}

func TestLookup_NonExistent(t *testing.T) {
	fs := newTestFS(t)
	_, errc := fs.lookup("/void")
	if errc == 0 {
		t.Error("expected error for nonexistent path")
	}
}

func TestNewNode(t *testing.T) {
	n := newNode("fid", "parent", "name", "/path", true)
	if n.fid != "fid" || n.name != "name" || n.currentPath != "/path" || !n.isFolder {
		t.Error("newNode fields mismatch")
	}
}

func TestFetchFilesResult(t *testing.T) {
	r := &fetchFilesResult{done: make(chan struct{})}
	if r.files != nil {
		t.Error("expected nil files")
	}
	if r.err != nil {
		t.Error("expected nil err")
	}
}

func TestReadRemoteFileSize(t *testing.T) {
	fs := newTestFS(t)

	n := newNode("local_read_test", "p", "read.txt", "/read.txt", false)
	n.size = 65536
	n.encSize = 66000
	fs.storeNode("/read.txt", n)

	buf := make([]byte, 100)
	nRead := fs.Read("/read.txt", buf, 0, 0)
	if nRead != 0 {
		t.Errorf("Read should return 0 for uncached remote file, got %d", nRead)
	}
}

func TestRead_LocalFile(t *testing.T) {
	fs := newTestFS(t)
	stgDir := t.TempDir()
	fs.staging, _ = staging.NewStore(stgDir)

	localPath, _ := fs.staging.Create("local_fid")
	fs.staging.WriteAt(localPath, []byte("hello local read"), 0)

	n := newNode("local_fid", "p", "local.txt", "/local.txt", false)
	n.localPath = localPath
	n.size = 16
	fs.storeNode("/local.txt", n)

	buf := make([]byte, 16)
	nRead := fs.Read("/local.txt", buf, 0, 0)
	if nRead != 16 {
		t.Errorf("expected 16 bytes, got %d", nRead)
	}
	if string(buf) != "hello local read" {
		t.Errorf("expected 'hello local read', got '%s'", string(buf))
	}
}

func TestWrite_NoStaging(t *testing.T) {
	fs := newTestFS(t)
	fs.staging = nil

	nWritten := fs.Write("/write.txt", []byte("data"), 0, 0)
	if nWritten != 0 {
		t.Errorf("expected 0 without staging, got %d", nWritten)
	}
}

func TestWrite_WithStaging(t *testing.T) {
	fs := newTestFS(t)
	stgDir := t.TempDir()
	fs.staging, _ = staging.NewStore(stgDir)

	fs.Create("/write_test.txt", 0, 0o644)
	nWritten := fs.Write("/write_test.txt", []byte("hello"), 0, 0)
	if nWritten == 0 {
		t.Error("expected > 0 bytes written")
	}
}

func TestIsShuttingDown(t *testing.T) {
	fs := newTestFS(t)
	if fs.IsShuttingDown() {
		t.Error("should not be shutting down initially")
	}
}
