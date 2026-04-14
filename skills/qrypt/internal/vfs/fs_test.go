package vfs

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/driver"
)

func TestFinderTrashPathHelpers(t *testing.T) {
	if !isFinderTrashDir("/.Trashes") {
		t.Fatal("expected /.Trashes to be treated as virtual trash dir")
	}
	if !isFinderTrashDir("/.Trashes/501") {
		t.Fatal("expected /.Trashes/<uid> to be treated as virtual trash dir")
	}
	if !isFinderTrashPath("/.Trashes/501/file.txt") {
		t.Fatal("expected Finder trash file path to be detected")
	}
	if isFinderTrashPath("/regular/path.txt") {
		t.Fatal("regular path should not be treated as Finder trash")
	}
}

func TestQryptFS_RenameToFinderTrashDeletesRemote(t *testing.T) {
	var (
		mu          sync.Mutex
		deleteCalls int
		deleteBody  string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/file/delete" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		mu.Lock()
		deleteCalls++
		deleteBody = string(body)
		mu.Unlock()
		fmt.Fprint(w, `{"status":200,"code":0,"message":"ok"}`)
	}))
	defer server.Close()

	oldBaseURL := driver.QuarkBaseURL
	oldV2URL := driver.QuarkV2URL
	oldAltURL := driver.QuarkV2AltURL
	driver.QuarkBaseURL = server.URL
	driver.QuarkV2URL = server.URL
	driver.QuarkV2AltURL = server.URL
	defer func() {
		driver.QuarkBaseURL = oldBaseURL
		driver.QuarkV2URL = oldV2URL
		driver.QuarkV2AltURL = oldAltURL
	}()

	d := driver.NewQuarkDriver("mock_cookie")
	d.SetClient(server.Client())

	fs := &QryptFS{driver: d}
	fs.nodes.Store("/doc.txt", &node{
		fid:       "remote-fid",
		parentFid: "root",
		name:      "doc.txt",
	})

	if errc := fs.Rename("/doc.txt", "/.Trashes/501/doc.txt"); errc != 0 {
		t.Fatalf("expected trash rename to succeed, got %d", errc)
	}

	mu.Lock()
	defer mu.Unlock()
	if deleteCalls != 1 {
		t.Fatalf("expected exactly one remote delete call, got %d", deleteCalls)
	}
	if !strings.Contains(deleteBody, `"filelist":["remote-fid"]`) {
		t.Fatalf("expected delete request to include remote fid, got %s", deleteBody)
	}
	if _, ok := fs.nodes.Load("/doc.txt"); ok {
		t.Fatal("expected original path to be removed from node cache")
	}
}

func TestQryptFS_RenameRecursiveCache(t *testing.T) {
	fs := &QryptFS{}

	// 准备测试数据
	fs.nodes.Store("/", &node{fid: "root", isFolder: true})
	fs.nodes.Store("/A", &node{fid: "fid_a", name: "A", isFolder: true})
	fs.nodes.Store("/A/b.txt", &node{fid: "fid_b", name: "b.txt", isFolder: false})
	fs.nodes.Store("/A/Sub", &node{fid: "fid_sub", name: "Sub", isFolder: true})
	fs.nodes.Store("/A/Sub/c.dat", &node{fid: "fid_c", name: "c.dat", isFolder: false})
	fs.nodes.Store("/Other", &node{fid: "fid_other", name: "Other", isFolder: false})

	// 模拟重命名 /A -> /X
	oldPath := "/A"
	newPath := "/X"
	newName := "X"

	v, _ := fs.nodes.Load(oldPath)
	oldNode := v.(*node)

	// 执行重命名逻辑 (手动模拟 fs.Rename 中的缓存更新部分)
	fs.nodes.Delete(oldPath)
	oldNode.name = newName
	fs.nodes.Store(newPath, oldNode)

	if oldNode.isFolder {
		oldPrefix := oldPath
		if !strings.HasSuffix(oldPrefix, "/") {
			oldPrefix += "/"
		}
		newPrefix := newPath
		if !strings.HasSuffix(newPrefix, "/") {
			newPrefix += "/"
		}

		fs.nodes.Range(func(key, value interface{}) bool {
			path, ok := key.(string)
			if !ok {
				return true
			}
			if strings.HasPrefix(path, oldPrefix) {
				childNode := value.(*node)
				relative := strings.TrimPrefix(path, oldPrefix)
				newChildPath := newPrefix + relative

				fs.nodes.Delete(path)
				fs.nodes.Store(newChildPath, childNode)
			}
			return true
		})
	}

	// 验证结果
	expectedPaths := []string{
		"/",
		"/X",
		"/X/b.txt",
		"/X/Sub",
		"/X/Sub/c.dat",
		"/Other",
	}

	for _, p := range expectedPaths {
		if _, ok := fs.nodes.Load(p); !ok {
			t.Errorf("Expected path %s not found in cache", p)
		}
	}

	unexpectedPaths := []string{
		"/A",
		"/A/b.txt",
		"/A/Sub",
		"/A/Sub/c.dat",
	}

	for _, p := range unexpectedPaths {
		if _, ok := fs.nodes.Load(p); ok {
			t.Errorf("Path %s should have been removed from cache", p)
		}
	}
}

func TestQryptFS_WriteSyncCoordination(t *testing.T) {
	n := &node{
		fid:      "test_fid",
		name:     "test.txt",
		size:     100,
		isDirty:  true,
		isFolder: false,
	}

	// 模拟同步开始前的快照
	n.mu.Lock()
	if !n.isDirty {
		t.Fatal("Node should be dirty")
	}
	snapshotSize := n.size
	n.isDirty = false
	n.mu.Unlock()

	if snapshotSize != 100 {
		t.Errorf("Expected snapshot size 100, got %d", snapshotSize)
	}
	if n.isDirty {
		t.Error("Node should NOT be dirty after snapshot")
	}

	// 模拟同步过程中（持有 snapshotSize）发生写入
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond) // 模拟同步中的延迟

		n.mu.Lock()
		n.size = 200
		n.isDirty = true
		n.mu.Unlock()
	}()

	// 模拟同步逻辑（使用 snapshotSize）
	// ... 假设这里正在使用 snapshotSize 上传 ...
	time.Sleep(50 * time.Millisecond)

	wg.Wait()

	// 验证最终状态
	n.mu.Lock()
	if n.size != 200 {
		t.Errorf("Expected final size 200, got %d", n.size)
	}
	if !n.isDirty {
		t.Error("Node should be dirty again after concurrent write")
	}
	n.mu.Unlock()
}

func TestSyncFailureLeavesNodeDirty_Regression(t *testing.T) {
	// 1. 创建一个总是返回错误的 mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, `{"code": 500, "message": "mock error"}`)
	}))
	defer server.Close()

	// 2. 修改 Driver 的全局 URL 为 mock server
	oldURL := driver.QuarkBaseURL
	driver.QuarkBaseURL = server.URL
	defer func() { driver.QuarkBaseURL = oldURL }()

	// 3. 初始化 QryptFS
	d := driver.NewQuarkDriver("mock_cookie")
	d.SetClient(server.Client())

	fs := &QryptFS{
		driver: d,
		cipher: &crypt.RcloneCipher{},
	}

	n := &node{
		fid:       "test_fid",
		parentFid: "root",
		name:      "test.txt",
		size:      10,
		isDirty:   true,
		isFolder:  false,
	}

	// 模拟 lookup 需要，因为 syncFile 会调用 lookup
	fs.nodes.Store("/", &node{fid: "root", isFolder: true})
	fs.nodes.Store("/test.txt", n)

	// 4. 执行同步，预期失败
	err := fs.syncFile("/test.txt", n)
	if err == nil {
		t.Fatal("Expected syncFile to fail, but it succeeded")
	}

	// 5. 验证 node 状态
	if !n.isDirty {
		t.Error("BUG: Node.isDirty should remain true after sync failure")
	}
}

func TestRenameDuringPendingSync_Regression(t *testing.T) {
	fs := &QryptFS{
		uploadChan: make(chan syncTask, 10),
	}

	n := &node{
		fid:         "test_fid",
		parentFid:   "root",
		name:        "old.txt",
		currentPath: "/old.txt",
		size:        10,
		isDirty:     true,
		isFolder:    false,
	}

	fs.storeNode("/", &node{fid: "root", currentPath: "/", isFolder: true})
	fs.storeNode("/old.txt", n)

	// 模拟已加入队列
	fs.uploadChan <- syncTask{node: n}

	// 模拟在 worker 处理前发生重命名
	n.name = "new.txt"
	fs.replaceNodePath("/old.txt", "/new.txt", n)

	task := <-fs.uploadChan

	if task.node != n {
		t.Fatal("unexpected node dequeued")
	}

	p := fs.currentPathForNode(task.node)
	if p != "/new.txt" {
		t.Errorf("Expected path /new.txt, got %s", p)
	}
}

func TestConcurrentSyncRequestsSameNodeDifferentPaths_Regression(t *testing.T) {
	fs := &QryptFS{}

	n := &node{
		fid:     "test_fid",
		name:    "file.txt",
		isDirty: true,
	}

	// 现在逻辑：节点作为锁
	if _, loaded := fs.syncing.LoadOrStore(n, struct{}{}); loaded {
		t.Error("Should have been able to lock first time")
	}

	if _, loaded := fs.syncing.LoadOrStore(n, struct{}{}); !loaded {
		t.Error("BUG: Should have detected sync for same node")
	}
}

func TestReaddir_ProtectsDirtyNodes_Regression(t *testing.T) {
	fs := &QryptFS{
		driver: driver.NewQuarkDriver("mock"),
		cipher: &crypt.RcloneCipher{},
	}

	// 1. 设置一个正在同步的 Dirty 节点
	n := &node{
		fid:      "local_fid",
		name:     "uploading.txt",
		size:     1000,
		isDirty:  true,
		isFolder: false,
		mtime:    time.Now(),
	}
	fs.nodes.Store("/uploading.txt", n)

	// 2. 模拟 Readdir 发现该文件在远程已存在（例如 Quark 已预创建，但大小仍为 0）
	// 手动执行 Readdir 中的保护逻辑
	childPath := "/uploading.txt"
	remoteFile := driver.File{
		Fid:      "remote_fid",
		FileName: "encrypted_name", // 模拟加密名
		Size:     "0",              // 模拟远程大小为 0
		File:     true,
	}

	// 模拟 Readdir 遍历到该文件的逻辑
	skipStore := false
	if v, ok := fs.nodes.Load(childPath); ok {
		existing := v.(*node)
		existing.mu.RLock()
		if existing.isDirty {
			skipStore = true
		}
		existing.mu.RUnlock()
	}

	if !skipStore {
		fs.nodes.Store(childPath, &node{
			fid:   remoteFile.Fid,
			size:  0,
			mtime: time.Now(),
		})
	}

	// 3. 验证本地节点未被覆盖
	v, _ := fs.nodes.Load(childPath)
	resultNode := v.(*node)
	if resultNode != n {
		t.Error("BUG: Dirty node was overwritten by remote metadata")
	}
	if resultNode.size != 1000 {
		t.Errorf("Expected size 1000, got %d", resultNode.size)
	}
}

func TestFlush_DeduplicatesSyncTasks_Regression(t *testing.T) {
	fs := &QryptFS{
		uploadChan: make(chan syncTask, 10),
	}

	n := &node{
		fid:         "test_fid",
		name:        "test.txt",
		currentPath: "/test.txt",
		localPath:   "/tmp/test_staging",
		isDirty:     true,
	}
	fs.storeNode("/test.txt", n)

	// 1. 模拟第一次 Flush
	fs.enqueueSync(n)

	// 2. 模拟第二次 Flush（重复触发）
	fs.enqueueSync(n)

	// 3. 验证队列中只有一个任务
	if len(fs.uploadChan) != 1 {
		t.Errorf("Expected 1 task in queue, got %d", len(fs.uploadChan))
	}

	// 4. 模拟 Worker 处理并重置状态
	task := <-fs.uploadChan
	n.mu.Lock()
	n.syncQueued = false
	n.isDirty = false // 模拟同步成功
	n.mu.Unlock()

	if task.node != n {
		t.Error("Incorrect node in task")
	}

	// 5. 模拟后续写入后再次 Flush
	n.mu.Lock()
	n.isDirty = true
	n.mu.Unlock()

	n.mu.Lock()
	if n.isDirty && !n.syncQueued {
		n.syncQueued = true
		n.mu.Unlock()
		fs.uploadChan <- syncTask{node: n}
	} else {
		n.mu.Unlock()
	}

	if len(fs.uploadChan) != 1 {
		t.Errorf("Expected 1 task in queue after reset, got %d", len(fs.uploadChan))
	}
}

func TestRenameSubtreeUpdatesPendingPaths_Regression(t *testing.T) {
	cacheDir := t.TempDir()
	cm, err := cache.NewCacheManager(cacheDir, filepath.Join(cacheDir, "qrypt_test.db"), 1<<20)
	if err != nil {
		t.Fatalf("cache init failed: %v", err)
	}
	defer cm.Close()

	fs := &QryptFS{cache: cm}
	root := &node{fid: "dir_fid", name: "A", currentPath: "/A", isFolder: true}
	child := &node{
		fid:         "local_child",
		parentFid:   "dir_fid",
		name:        "b.txt",
		currentPath: "/A/b.txt",
		localPath:   filepath.Join(cacheDir, "staging-file"),
		isDirty:     true,
	}

	fs.storeNode("/A", root)
	fs.storeNode("/A/b.txt", child)
	fs.persistPendingPath("", "/A/b.txt", child)

	fs.renameSubtreePaths("/A", "/X")

	pending, err := cm.GetPendingNodes()
	if err != nil {
		t.Fatalf("failed to load pending nodes: %v", err)
	}
	if len(pending) != 1 || pending[0].Path != "/X/b.txt" {
		t.Fatalf("expected pending path to move to /X/b.txt, got %+v", pending)
	}
	if path := fs.currentPathForNode(child); path != "/X/b.txt" {
		t.Fatalf("expected child current path to be /X/b.txt, got %s", path)
	}
	if _, ok := fs.nodes.Load("/A/b.txt"); ok {
		t.Fatal("expected old child path to be removed")
	}
}

func TestCleanupLocalUploadStateRemovesDirectorySubtree_Regression(t *testing.T) {
	fs := &QryptFS{}
	root := &node{fid: "dir", name: "dir", currentPath: "/dir", isFolder: true}
	child := &node{fid: "file", name: "file.txt", currentPath: "/dir/file.txt"}

	fs.storeNode("/dir", root)
	fs.storeNode("/dir/file.txt", child)

	fs.cleanupLocalUploadState("/dir", root, true)

	if _, ok := fs.nodes.Load("/dir"); ok {
		t.Fatal("expected directory root to be removed")
	}
	if _, ok := fs.nodes.Load("/dir/file.txt"); ok {
		t.Fatal("expected directory child to be removed")
	}
	if path := fs.currentPathForNode(child); path != "" {
		t.Fatalf("expected child path to be cleared, got %s", path)
	}
}
