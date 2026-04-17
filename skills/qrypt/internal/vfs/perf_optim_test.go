package vfs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/driver"
)

// ---------------------------------------------------------------------------
// mockServerWithCounter — 记录每个 API path 的调用次数
// ---------------------------------------------------------------------------

type mockServerWithCounter struct {
	server      *httptest.Server
	callCounts  sync.Map // path -> *atomic.Int64
	responseMap sync.Map // path -> handler
}

func newMockServerWithCounter() *mockServerWithCounter {
	m := &mockServerWithCounter{}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Path
		val, _ := m.callCounts.LoadOrStore(key, &atomic.Int64{})
		val.(*atomic.Int64).Add(1)
		if v, ok := m.responseMap.Load(key); ok {
			v.(http.HandlerFunc)(w, r)
			return
		}
		http.Error(w, fmt.Sprintf("unhandled path: %s", key), http.StatusNotFound)
	}))
	return m
}

func (m *mockServerWithCounter) handle(path string, fn http.HandlerFunc) {
	m.responseMap.Store(path, fn)
}

func (m *mockServerWithCounter) count(path string) int64 {
	if v, ok := m.callCounts.Load(path); ok {
		return v.(*atomic.Int64).Load()
	}
	return 0
}

func (m *mockServerWithCounter) reset() {
	m.callCounts.Range(func(key, value interface{}) bool {
		value.(*atomic.Int64).Store(0)
		return true
	})
}

func (m *mockServerWithCounter) close() {
	m.server.Close()
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

type sortResp struct {
	driver.Resp
	Data struct {
		List []driver.File `json:"list"`
	} `json:"data"`
	Metadata struct {
		Total int `json:"_total"`
	} `json:"metadata"`
}

func makeDirResponse(files []driver.File) string {
	resp := sortResp{}
	resp.Status = 200
	resp.Code = 0
	resp.Data.List = files
	resp.Metadata.Total = len(files)
	b, _ := json.Marshal(resp)
	return string(b)
}

// testEntry is a shorthand for defining file/dir entries in tests.
type testEntry struct {
	fid   string
	name  string
	isDir bool
	size  int64
}

func encName(cipher *crypt.RcloneCipher, name string) string {
	return cipher.EncryptSegment(name)
}

func makeEncryptedFiles(cipher *crypt.RcloneCipher, entries []testEntry) []driver.File {
	files := make([]driver.File, len(entries))
	for i, e := range entries {
		files[i] = driver.File{
			Fid:      e.fid,
			FileName: encName(cipher, e.name),
			Size:     json.Number(fmt.Sprintf("%d", e.size)),
			File:     !e.isDir,
		}
	}
	return files
}

func setupPerfTestFS(t *testing.T, m *mockServerWithCounter, cipher *crypt.RcloneCipher,
	rootEntries []testEntry,
	childEntries map[string][]testEntry,
) *QryptFS {
	t.Helper()

	rootFiles := makeEncryptedFiles(cipher, rootEntries)
	childDirs := make(map[string][]driver.File)
	for fid, entries := range childEntries {
		childDirs[fid] = makeEncryptedFiles(cipher, entries)
	}

	m.handle("/file/sort", func(w http.ResponseWriter, r *http.Request) {
		pdirFid := r.URL.Query().Get("pdir_fid")
		if pdirFid == "root_fid" {
			fmt.Fprint(w, makeDirResponse(rootFiles))
			return
		}
		if files, ok := childDirs[pdirFid]; ok {
			fmt.Fprint(w, makeDirResponse(files))
			return
		}
		fmt.Fprint(w, makeDirResponse(nil))
	})

	d := driver.NewQuarkDriver("mock_cookie")
	d.SetClient(m.server.Client())
	driver.QuarkBaseURL = m.server.URL
	driver.QuarkV2URL = m.server.URL
	driver.QuarkV2AltURL = m.server.URL

	fs := &QryptFS{
		driver:  d,
		cipher:  cipher,
		rootFid: "root_fid",
	}
	fs.nodes.Store("/", &node{
		fid:               "root_fid",
		name:              "/",
		currentPath:       "/",
		isFolder:          true,
		lastMetadataCheck: time.Time{},
		children:          make(map[string]*node),
	})
	return fs
}

func newTestCipher(t *testing.T) *crypt.RcloneCipher {
	t.Helper()
	c, err := crypt.NewRcloneCipher("testpass", "")
	if err != nil {
		t.Fatalf("cipher init: %v", err)
	}
	return c
}

// ---------------------------------------------------------------------------
// 测试 1：Readdir 后同目录 Getattr 不应重复调用 ListFiles
// ---------------------------------------------------------------------------

func TestPerfOptim_LookupAfterReaddir_NoRedundantListFiles(t *testing.T) {
	m := newMockServerWithCounter()
	defer m.close()
	cipher := newTestCipher(t)

	rootEntries := []testEntry{
		{fid: "f1", name: "alpha.txt", size: 100},
		{fid: "f2", name: "beta.txt", size: 200},
		{fid: "d1", name: "subdir", isDir: true},
	}
	fs := setupPerfTestFS(t, m, cipher, rootEntries, nil)

	fill := func(name string, stat *fuse.Stat_t, ofst int64) bool { return true }

	// 第一次 Readdir — 1 次 ListFiles
	fs.Readdir("/", fill, 0, 0)
	if c := m.count("/file/sort"); c != 1 {
		t.Fatalf("expected 1 ListFiles call after first Readdir, got %d", c)
	}

	// 验证 lastMetadataCheck 已刷新
	rootV, _ := fs.nodes.Load("/")
	root := rootV.(*node)
	root.mu.RLock()
	fresh := time.Since(root.lastMetadataCheck) < 5*time.Second
	childCount := len(root.children)
	root.mu.RUnlock()
	if !fresh {
		t.Error("expected root.lastMetadataCheck to be fresh after Readdir")
	}
	t.Logf("children populated by Readdir: %d", childCount)
	if childCount == 0 {
		t.Fatal("Readdir did not populate children — encryption may be broken")
	}

	// 对 Readdir 已加载的 children 调 Getattr，不应再触发 API
	m.reset()
	root.mu.RLock()
	for name := range root.children {
		fs.Getattr("/"+name, &fuse.Stat_t{}, 0)
	}
	root.mu.RUnlock()

	redundantCalls := m.count("/file/sort")
	if redundantCalls > 0 {
		t.Errorf("OPTIMIZATION: Getattr on %d known children triggered %d redundant ListFiles calls (expected 0)",
			childCount, redundantCalls)
	}
}

// ---------------------------------------------------------------------------
// 测试 2：子目录预取
// ---------------------------------------------------------------------------

func TestPerfOptim_ReaddirPrefetchesSubdirectories(t *testing.T) {
	m := newMockServerWithCounter()
	defer m.close()
	cipher := newTestCipher(t)

	rootEntries := []testEntry{
		{fid: "dir_a", name: "docs", isDir: true},
		{fid: "dir_b", name: "images", isDir: true},
		{fid: "f1", name: "readme.txt", size: 100},
	}
	childEntries := map[string][]testEntry{
		"dir_a": {{fid: "a1", name: "spec.txt", size: 50}},
		"dir_b": {{fid: "b1", name: "photo.jpg", size: 70}},
	}
	fs := setupPerfTestFS(t, m, cipher, rootEntries, childEntries)
	fill := func(name string, stat *fuse.Stat_t, ofst int64) bool { return true }

	fs.Readdir("/", fill, 0, 0)
	rootCalls := m.count("/file/sort")
	t.Logf("ListFiles calls after Readdir /: %d", rootCalls)

	// 等待可能的后台预取
	time.Sleep(2 * time.Second)
	totalCalls := m.count("/file/sort")
	t.Logf("ListFiles calls after 2s wait: %d", totalCalls)

	if totalCalls <= rootCalls {
		t.Log("OPTIMIZATION: subdirectories were NOT pre-fetched after Readdir")
		t.Log("Implement background prefetch: concurrently ListFiles child directories")
	} else {
		t.Logf("PREFETCH DETECTED: %d extra calls for subdirs", totalCalls-rootCalls)
	}

	// 进入子目录 — 如果有预取缓存不应再调 API
	m.reset()
	fs.Readdir("/docs", fill, 0, 0)
	dirACalls := m.count("/file/sort")
	t.Logf("Readdir /docs ListFiles calls: %d", dirACalls)
}

// ---------------------------------------------------------------------------
// 测试 3：大目录 lookup 性能
// ---------------------------------------------------------------------------

func TestPerfOptim_LookupInLargeDirectory(t *testing.T) {
	m := newMockServerWithCounter()
	defer m.close()
	cipher := newTestCipher(t)

	rootEntries := make([]testEntry, 50)
	for i := 0; i < 50; i++ {
		rootEntries[i] = testEntry{fid: fmt.Sprintf("fid_%d", i), name: fmt.Sprintf("file_%03d.txt", i), size: 100}
	}
	fs := setupPerfTestFS(t, m, cipher, rootEntries, nil)
	fill := func(name string, stat *fuse.Stat_t, ofst int64) bool { return true }

	fs.Readdir("/", fill, 0, 0)

	rootV, _ := fs.nodes.Load("/")
	root := rootV.(*node)
	root.mu.RLock()
	childCount := len(root.children)
	root.mu.RUnlock()
	t.Logf("children count: %d", childCount)

	if childCount == 0 {
		t.Fatal("no children populated")
	}

	// benchmark lookup
	start := time.Now()
	iterations := 100
	root.mu.RLock()
	for i := 0; i < iterations; i++ {
		for name := range root.children {
			fs.lookup("/" + name)
		}
	}
	root.mu.RUnlock()
	elapsed := time.Since(start)
	perLookup := elapsed / time.Duration(iterations*childCount)
	t.Logf("lookup %d children x %d iterations: total=%v, per-lookup=%v",
		childCount, iterations, elapsed, perLookup)

	if perLookup > 50*time.Microsecond {
		t.Logf("OPTIMIZATION: lookup took %v per call (>50µs), map index should be <5µs", perLookup)
	}
}

// ---------------------------------------------------------------------------
// 测试 4：连续 stat 同目录文件 — 不应穿透到 API
// ---------------------------------------------------------------------------

func TestPerfOptim_StatMultipleFilesSameDir_MinimalAPICalls(t *testing.T) {
	m := newMockServerWithCounter()
	defer m.close()
	cipher := newTestCipher(t)

	rootEntries := []testEntry{
		{fid: "f1", name: "alpha.txt", size: 100},
		{fid: "f2", name: "beta.txt", size: 200},
		{fid: "f3", name: "gamma.txt", size: 300},
	}
	fs := setupPerfTestFS(t, m, cipher, rootEntries, nil)
	fill := func(name string, stat *fuse.Stat_t, ofst int64) bool { return true }

	fs.Readdir("/", fill, 0, 0)
	t.Logf("ListFiles after Readdir: %d", m.count("/file/sort"))

	m.reset()
	rootV, _ := fs.nodes.Load("/")
	root := rootV.(*node)
	root.mu.RLock()
	for name := range root.children {
		fs.Getattr("/"+name, &fuse.Stat_t{}, 0)
	}
	root.mu.RUnlock()

	statCalls := m.count("/file/sort")
	t.Logf("Getattr x%d -> ListFiles: %d", len(root.children), statCalls)
	if statCalls > 0 {
		t.Errorf("OPTIMIZATION: Getattr within TTL triggered %d ListFiles calls (expected 0)", statCalls)
	}

	// 1s 后再 stat — 还在 TTL 内
	time.Sleep(1 * time.Second)
	m.reset()
	root.mu.RLock()
	for name := range root.children {
		fs.Getattr("/"+name, &fuse.Stat_t{}, 0)
	}
	root.mu.RUnlock()

	statCalls2 := m.count("/file/sort")
	t.Logf("Getattr round 2 -> ListFiles: %d", statCalls2)
	if statCalls2 > 0 {
		t.Errorf("OPTIMIZATION: Getattr within TTL round 2 triggered %d ListFiles calls", statCalls2)
	}
}

// ---------------------------------------------------------------------------
// 测试 5：fileExistsOnServer 每次都调 ListFiles
// ---------------------------------------------------------------------------

func TestPerfOptim_FileExistsOnServer_CachesResult(t *testing.T) {
	m := newMockServerWithCounter()
	defer m.close()
	cipher := newTestCipher(t)

	rootEntries := []testEntry{
		{fid: "f1", name: "doc.txt", size: 100},
	}
	fs := setupPerfTestFS(t, m, cipher, rootEntries, nil)

	docNode := &node{
		fid: "f1", parentFid: "root_fid", name: "doc.txt",
		currentPath: "/doc.txt", isDirty: false, lastMetadataCheck: time.Now(),
	}
	fs.storeNode("/doc.txt", docNode)

	// Ensure root's lastMetadataCheck is fresh so the optimization kicks in
	if rootV, ok := fs.nodes.Load("/"); ok {
		root := rootV.(*node)
		root.mu.Lock()
		root.lastMetadataCheck = time.Now()
		root.mu.Unlock()
		// setupPerfTestFS uses direct Store, not storeNode — manually populate fidNodes
		fs.fidNodes.Store("root_fid", root)
	}

	m.reset()
	for i := 0; i < 5; i++ {
		fs.fileExistsOnServer("f1", "root_fid")
	}

	calls := m.count("/file/sort")
	t.Logf("fileExistsOnServer x5 -> ListFiles calls: %d", calls)
	if calls >= 5 {
		t.Errorf("OPTIMIZATION: fileExistsOnServer does RemoveDirCache+ListFiles each time (%d calls)", calls)
	}
}

// ---------------------------------------------------------------------------
// 测试 6：典型工作流总 API 调用次数
// ---------------------------------------------------------------------------

func TestPerfOptim_TypicalWorkflow_TotalAPICalls(t *testing.T) {
	m := newMockServerWithCounter()
	defer m.close()
	cipher := newTestCipher(t)

	rootEntries := []testEntry{
		{fid: "dir_docs", name: "docs", isDir: true},
		{fid: "dir_imgs", name: "images", isDir: true},
		{fid: "f1", name: "readme.txt", size: 500},
		{fid: "f2", name: "main.go", size: 1000},
	}
	childEntries := map[string][]testEntry{
		"dir_docs": {
			{fid: "d1", name: "spec.md", size: 200},
			{fid: "d2", name: "design.md", size: 300},
		},
		"dir_imgs": {},
	}
	fs := setupPerfTestFS(t, m, cipher, rootEntries, childEntries)
	fill := func(name string, stat *fuse.Stat_t, ofst int64) bool { return true }

	// Step 1: ls /
	m.reset()
	fs.Readdir("/", fill, 0, 0)
	step1 := m.count("/file/sort")

	// Step 2: stat all root children
	m.reset()
	rootV, _ := fs.nodes.Load("/")
	root := rootV.(*node)
	root.mu.RLock()
	for name := range root.children {
		fs.Getattr("/"+name, &fuse.Stat_t{}, 0)
	}
	root.mu.RUnlock()
	step2 := m.count("/file/sort")

	// Step 3: ls /docs
	m.reset()
	fs.Readdir("/docs", fill, 0, 0)
	step3 := m.count("/file/sort")

	// Step 4: stat docs children
	m.reset()
	docsV, _ := fs.nodes.Load("/docs")
	docs := docsV.(*node)
	docs.mu.RLock()
	for name := range docs.children {
		fs.Getattr("/docs/"+name, &fuse.Stat_t{}, 0)
	}
	docs.mu.RUnlock()
	step4 := m.count("/file/sort")

	total := step1 + step2 + step3 + step4
	t.Logf("Workflow: ls/=%d stat_root=%d ls_docs=%d stat_docs=%d total=%d",
		step1, step2, step3, step4, total)

	ideal := int64(2) // step1=1, step2=0, step3=1, step4=0
	if total > ideal {
		t.Logf("OPTIMIZATION: ideal=%d, actual=%d, %d redundant calls", ideal, total, total-ideal)
	}
}

// ---------------------------------------------------------------------------
// 测试 7：FUSE 内核缓存参数
// ---------------------------------------------------------------------------

func TestPerfOptim_MountOptions_IncludeFUSECache(t *testing.T) {
	opts := MountOptions()
	optsStr := strings.Join(opts, " ")

	if !strings.Contains(optsStr, "attr_timeout") {
		t.Log("OPTIMIZATION: mount options missing 'attr_timeout' — kernel won't cache attributes")
		t.Log("Suggested: add '-o', 'attr_timeout=60'")
	}
	if !strings.Contains(optsStr, "entry_timeout") {
		t.Log("OPTIMIZATION: mount options missing 'entry_timeout' — kernel won't cache directory entries")
		t.Log("Suggested: add '-o', 'entry_timeout=60'")
	}
}

// ---------------------------------------------------------------------------
// 测试 8：深层路径 lookup — 缓存后不应重复调 API
// ---------------------------------------------------------------------------

func TestPerfOptim_DeepPathLookup_CachedAfterFirstCall(t *testing.T) {
	m := newMockServerWithCounter()
	defer m.close()
	cipher := newTestCipher(t)

	rootEntries := []testEntry{{fid: "dir_a", name: "a", isDir: true}}
	childEntries := map[string][]testEntry{
		"dir_a": {{fid: "dir_b", name: "b", isDir: true}},
		"dir_b": {{fid: "file_c", name: "c.txt", size: 42}},
	}
	fs := setupPerfTestFS(t, m, cipher, rootEntries, childEntries)

	// 冷启动
	m.reset()
	fs.lookup("/a/b/c.txt")
	coldCalls := m.count("/file/sort")
	t.Logf("Cold deep lookup: %d ListFiles calls", coldCalls)

	// 热 lookup — 应缓存
	m.reset()
	fs.lookup("/a/b/c.txt")
	warmCalls := m.count("/file/sort")
	t.Logf("Warm deep lookup: %d ListFiles calls", warmCalls)
	if warmCalls > 0 {
		t.Errorf("OPTIMIZATION: warm lookup triggered %d ListFiles calls (expected 0)", warmCalls)
	}
}

// ---------------------------------------------------------------------------
// 测试 9：TTL 过期后发现文件被远端删除 — 应返回 ENOENT 并清理缓存
// ---------------------------------------------------------------------------

func TestPerfOptim_RemoteDelete_ReturnsENOENT(t *testing.T) {
	m := newMockServerWithCounter()
	defer m.close()
	cipher := newTestCipher(t)

	// 初始：root 下有两个文件
	rootEntries := []testEntry{
		{fid: "f1", name: "alpha.txt", size: 100},
		{fid: "f2", name: "beta.txt", size: 200},
	}
	fs := setupPerfTestFS(t, m, cipher, rootEntries, nil)

	// 模拟 Readdir 加载目录
	fill := func(name string, stat *fuse.Stat_t, ofst int64) bool { return true }
	fs.Readdir("/", fill, 0, 0)

	// 确认 alpha.txt 存在于缓存
	if _, errc := fs.lookup("/alpha.txt"); errc != 0 {
		t.Fatal("alpha.txt should exist in cache after Readdir")
	}

	// 模拟远端删除 alpha.txt — mock 只返回 beta.txt
	remainingEntries := []testEntry{
		{fid: "f2", name: "beta.txt", size: 200},
	}
	remainingFiles := makeEncryptedFiles(cipher, remainingEntries)
	m.handle("/file/sort", func(w http.ResponseWriter, r *http.Request) {
		pdirFid := r.URL.Query().Get("pdir_fid")
		if pdirFid == "root_fid" {
			fmt.Fprint(w, makeDirResponse(remainingFiles))
			return
		}
		fmt.Fprint(w, makeDirResponse(nil))
	})
	// 清除驱动内部的 dirCache，确保下次 ListFiles 请求到新 mock
	fs.driver.RemoveDirCache("root_fid")

	// 让 alpha.txt 的 lastMetadataCheck 过期（> MetadataTTL）
	alphaV, _ := fs.nodes.Load("/alpha.txt")
	alpha := alphaV.(*node)
	alpha.mu.Lock()
	alpha.lastMetadataCheck = time.Now().Add(-MetadataTTL - 10*time.Second)
	alpha.mu.Unlock()

	// lookup 应返回 ENOENT（触发 TTL 刷新，发现文件已删除）
	_, errc := fs.lookup("/alpha.txt")
	if errc == 0 {
		t.Error("expected ENOENT for remotely deleted alpha.txt, got success")
	}
	if errc != -fuse.ENOENT {
		t.Errorf("expected ENOENT (-%d), got %d", fuse.ENOENT, errc)
	}

	// 节点应从缓存中移除
	if _, ok := fs.nodes.Load("/alpha.txt"); ok {
		t.Error("alpha.txt should be removed from cache after deletion detected")
	}

	// beta.txt 仍应正常访问
	if _, errc := fs.lookup("/beta.txt"); errc != 0 {
		t.Errorf("beta.txt should still be accessible, got errc=%d", errc)
	}

	t.Log("PASS: remote delete detected, ENOENT returned, cache cleaned")
}

// ---------------------------------------------------------------------------
// 测试 10：Rename 跨目录移动 — Move 23008 瞬态错误重试后成功
// ---------------------------------------------------------------------------

func TestRename_RetryMove23008_SucceedsOnRetry(t *testing.T) {
	m := newMockServerWithCounter()
	defer m.close()
	cipher := newTestCipher(t)

	rootEntries := []testEntry{{fid: "dir_test", name: "test", isDir: true}}
	childEntries := map[string][]testEntry{
		"dir_test": {{fid: "f1", name: "old_name.txt", size: 1024}},
	}
	fs := setupPerfTestFS(t, m, cipher, rootEntries, childEntries)

	// 模拟 Readdir 预热缓存
	fill := func(name string, stat *fuse.Stat_t, ofst int64) bool { return true }
	fs.Readdir("/", fill, 0, 0)
	fs.Readdir("/test", fill, 0, 0)

	// Move: 第一次返回 23008，第二次成功
	moveAttempts := 0
	m.handle("/file/move", func(w http.ResponseWriter, r *http.Request) {
		moveAttempts++
		if moveAttempts == 1 {
			fmt.Fprint(w, `{"status":400,"code":23008,"message":"file is doloading[同名冲突]"}`)
			return
		}
		fmt.Fprint(w, `{"status":200,"code":0}`)
	})

	// Rename: 始终成功
	renameAttempts := 0
	m.handle("/file/rename", func(w http.ResponseWriter, r *http.Request) {
		renameAttempts++
		fmt.Fprint(w, `{"status":200,"code":0}`)
	})

	// 执行 Rename: /test/old_name.txt → /new_name.txt
	errc := fs.Rename("/test/old_name.txt", "/new_name.txt")
	if errc != 0 {
		t.Fatalf("expected success (0), got %d", errc)
	}

	// 验证 Move 被调用了 2 次（第一次失败，重试成功）
	if moveAttempts != 2 {
		t.Errorf("expected 2 Move attempts, got %d", moveAttempts)
	}

	// 验证 Rename 被调用了 1 次
	if renameAttempts != 1 {
		t.Errorf("expected 1 Rename attempt, got %d", renameAttempts)
	}

	// 验证节点已移动到新路径
	if _, errc := fs.lookup("/new_name.txt"); errc != 0 {
		t.Error("new_name.txt should exist at new path")
	}
	if _, ok := fs.nodes.Load("/test/old_name.txt"); ok {
		t.Error("/test/old_name.txt should be removed from cache")
	}

	t.Log("PASS: Move 23008 retried, Rename succeeded")
}

// ---------------------------------------------------------------------------
// 测试 11：Rename 跨目录移动 — Move 成功，Rename 23008 重试后成功
// ---------------------------------------------------------------------------

func TestRename_RetryRename23008_SucceedsOnRetry(t *testing.T) {
	m := newMockServerWithCounter()
	defer m.close()
	cipher := newTestCipher(t)

	rootEntries := []testEntry{{fid: "dir_test", name: "test", isDir: true}}
	childEntries := map[string][]testEntry{
		"dir_test": {{fid: "f1", name: "old_name.txt", size: 1024}},
	}
	fs := setupPerfTestFS(t, m, cipher, rootEntries, childEntries)

	fill := func(name string, stat *fuse.Stat_t, ofst int64) bool { return true }
	fs.Readdir("/", fill, 0, 0)
	fs.Readdir("/test", fill, 0, 0)

	// Move: 始终成功
	moveAttempts := 0
	m.handle("/file/move", func(w http.ResponseWriter, r *http.Request) {
		moveAttempts++
		fmt.Fprint(w, `{"status":200,"code":0}`)
	})

	// Rename: 第一次返回 23008，第二次成功
	renameAttempts := 0
	m.handle("/file/rename", func(w http.ResponseWriter, r *http.Request) {
		renameAttempts++
		if renameAttempts == 1 {
			fmt.Fprint(w, `{"status":400,"code":23008,"message":"file is doloading[同名冲突]"}`)
			return
		}
		fmt.Fprint(w, `{"status":200,"code":0}`)
	})

	errc := fs.Rename("/test/old_name.txt", "/new_name.txt")
	if errc != 0 {
		t.Fatalf("expected success (0), got %d", errc)
	}

	if moveAttempts != 1 {
		t.Errorf("expected 1 Move attempt, got %d", moveAttempts)
	}
	if renameAttempts != 2 {
		t.Errorf("expected 2 Rename attempts, got %d", renameAttempts)
	}

	t.Log("PASS: Rename 23008 retried, operation succeeded")
}

// ---------------------------------------------------------------------------
// 测试 12：Rename 非可重试错误 — 立即返回 EIO
// ---------------------------------------------------------------------------

func TestRename_NonRetryableError_ReturnsEIO(t *testing.T) {
	m := newMockServerWithCounter()
	defer m.close()
	cipher := newTestCipher(t)

	rootEntries := []testEntry{{fid: "dir_test", name: "test", isDir: true}}
	childEntries := map[string][]testEntry{
		"dir_test": {{fid: "f1", name: "old_name.txt", size: 1024}},
	}
	fs := setupPerfTestFS(t, m, cipher, rootEntries, childEntries)

	fill := func(name string, stat *fuse.Stat_t, ofst int64) bool { return true }
	fs.Readdir("/", fill, 0, 0)
	fs.Readdir("/test", fill, 0, 0)

	// Move: 返回不可重试的错误
	moveAttempts := 0
	m.handle("/file/move", func(w http.ResponseWriter, r *http.Request) {
		moveAttempts++
		fmt.Fprint(w, `{"status":403,"code":403,"message":"permission denied"}`)
	})

	m.handle("/file/rename", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":200,"code":0}`)
	})

	errc := fs.Rename("/test/old_name.txt", "/new_name.txt")
	if errc == 0 {
		t.Fatal("expected EIO for permission denied, got success")
	}

	// 不可重试错误只应调用 1 次
	if moveAttempts != 1 {
		t.Errorf("expected 1 Move attempt (no retry), got %d", moveAttempts)
	}

	t.Log("PASS: non-retryable error returned EIO immediately")
}
