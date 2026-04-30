package vfs

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/driver"
	"github.com/yinzhenyu/skills/qrypt/internal/staging"
	uploadpkg "github.com/yinzhenyu/skills/qrypt/internal/upload"
)

// ghostTestTransport 是一个完整的 mock HTTP transport，
// 支持上传流程的所有端点 + ListFiles/Delete，同时记录请求。
type ghostTestTransport struct {
	partLatency    time.Duration // 上传每个 part 的延迟（用于模拟慢上传）
	controlLatency time.Duration

	mu       sync.Mutex
	requests []trackedRequest
	uploads  int // 已上传的 part 数量
}

type trackedRequest struct {
	Method string
	Path   string
	Body   string
	Time   time.Time
}

func (t *ghostTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		req.Body.Close()
	}

	t.mu.Lock()
	t.requests = append(t.requests, trackedRequest{
		Method: req.Method,
		Path:   req.URL.Path,
		Body:   string(body),
		Time:   time.Now(),
	})
	t.mu.Unlock()

	path := req.URL.Path
	query := req.URL.Query()

	switch {
	// ListFiles (file/sort)
	case req.Method == http.MethodGet && strings.HasSuffix(path, "/file/sort"):
		return jsonResp(200, map[string]any{
			"data":     map[string]any{"list": []any{}},
			"metadata": map[string]any{"_total": 0},
		}), nil

	// Delete
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/file/delete"):
		return jsonResp(200, map[string]any{"message": "ok"}), nil

	// UploadPre
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/file/upload/pre"):
		time.Sleep(t.controlLatency)
		return jsonResp(200, map[string]any{
			"data": map[string]any{
				"task_id":    "task-1",
				"upload_id":  "upload-1",
				"obj_key":    "obj-1",
				"upload_url": "https://mock-oss.local/upload",
				"fid":        "server-fid-from-pre",
				"finish":     false,
				"bucket":     "bucket-1",
				"callback":   map[string]any{"callbackUrl": "cb", "callbackBody": "ok"},
				"auth_info":  "auth",
			},
			"metadata": map[string]any{"part_size": 8 * 1024 * 1024},
		}), nil

	// UploadAuth
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/file/upload/auth"):
		time.Sleep(t.controlLatency)
		return jsonResp(200, map[string]any{
			"data": map[string]any{"auth_key": "mock-auth"},
		}), nil

	// UpdateHash
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/file/update/hash"):
		time.Sleep(t.controlLatency)
		return jsonResp(200, map[string]any{
			"data": map[string]any{"finish": false, "fid": "server-fid-from-pre"},
		}), nil

	// UploadFinish
	case req.Method == http.MethodPost && strings.HasSuffix(path, "/file/upload/finish"):
		time.Sleep(t.controlLatency)
		return jsonResp(200, map[string]any{"message": "ok"}), nil

	// UploadPart (PUT with partNumber)
	case req.Method == http.MethodPut && query.Get("partNumber") != "":
		partNum, _ := strconv.Atoi(query.Get("partNumber"))
		_ = partNum
		// 模拟上传延迟
		time.Sleep(t.partLatency)
		t.mu.Lock()
		t.uploads++
		t.mu.Unlock()
		resp := emptyResp(200)
		resp.Header.Set("Etag", fmt.Sprintf("etag-%d", partNum))
		return resp, nil

	// Commit (POST with uploadId)
	case req.Method == http.MethodPost && query.Get("uploadId") != "":
		time.Sleep(t.controlLatency)
		return emptyResp(200), nil

	default:
		return jsonResp(404, map[string]any{"message": "not mocked"}), nil
	}
}

func (t *ghostTestTransport) hasRequest(path string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, r := range t.requests {
		if strings.Contains(r.Path, path) {
			return true
		}
	}
	return false
}

func (t *ghostTestTransport) countRequests(path string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	count := 0
	for _, r := range t.requests {
		if strings.Contains(r.Path, path) {
			count++
		}
	}
	return count
}

func (t *ghostTestTransport) getUploadCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.uploads
}

func (t *ghostTestTransport) dumpRequests(logf func(string, ...any)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	logf("=== Request Log (%d total) ===", len(t.requests))
	for i, r := range t.requests {
		logf("  [%d] %s %s (t=%s)", i, r.Method, r.Path, r.Time.Format("15:04:05.000"))
	}
	logf("=== End Request Log ===")
}

// --- HTTP 响应辅助函数 ---

func jsonResp(status int, data map[string]any) *http.Response {
	// 简化：直接构造 JSON 字符串
	var parts []string
	for k, v := range data {
		parts = append(parts, fmt.Sprintf("%q:%v", k, formatJSON(v)))
	}
	body := "{" + strings.Join(parts, ",") + "}"
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func formatJSON(v any) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case int:
		return strconv.Itoa(val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case map[string]any:
		var parts []string
		for k, v2 := range val {
			parts = append(parts, fmt.Sprintf("%q:%v", k, formatJSON(v2)))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case []any:
		var parts []string
		for _, item := range val {
			parts = append(parts, formatJSON(item))
		}
		return "[" + strings.Join(parts, ",") + "]"
	default:
		return fmt.Sprintf("%v", val)
	}
}

func emptyResp(status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
	}
}

// --- 测试辅助 ---

func buildGhostTestFS(t *testing.T, transport *ghostTestTransport) *QryptFS {
	t.Helper()

	d := driver.NewQuarkDriver("mock_cookie")
	d.SetClient(&http.Client{Transport: transport})

	cipher, err := crypt.NewRcloneCipher("test-password", "")
	if err != nil {
		t.Fatalf("cipher init failed: %v", err)
	}

	cacheDir := t.TempDir()
	cm, err := cache.NewCacheManager(cacheDir, filepath.Join(cacheDir, "ghost_test.db"), 1<<30)
	if err != nil {
		t.Fatalf("cache init failed: %v", err)
	}
	t.Cleanup(func() { cm.Close() })

	stagingDir := filepath.Join(cacheDir, "staging")
	stagingStore, err := staging.NewStore(stagingDir)
	if err != nil {
		t.Fatalf("staging init failed: %v", err)
	}

	fs := &QryptFS{
		driver:         d,
		cache:          cm,
		rootFid:        "root_fid",
		cipher:         cipher,
		uploadChan:     make(chan syncTask, 100),
		metadataOpChan: make(chan metadataTask, 100),
		opsLogChan:     make(chan metadataTask, 100),
		prefetchSem:    make(chan struct{}, 30),
		staging:        stagingStore,
		uploader:       uploadpkg.NewManager(d, cipher, stagingStore),
		maxRetries:     1,
	}

	fs.storeNode("/", &node{
		fid: "root_fid", name: "", parentFid: "0",
		currentPath: "/", isFolder: true, mtime: time.Now(), lastReadBlock: -1,
	})

	// 只启动 1 个 uploadWorker，不启动 metadataWorker（避免干扰）
	go fs.uploadWorker()

	return fs
}

func createGhostTestFile(t *testing.T, fs *QryptFS, path, fid, parentFid string, size int) *node {
	t.Helper()

	localPath, err := fs.staging.Create(fid)
	if err != nil {
		t.Fatalf("staging create failed: %v", err)
	}

	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 251)
	}
	if _, err := fs.staging.WriteAt(localPath, data, 0); err != nil {
		t.Fatalf("staging write failed: %v", err)
	}

	name := filepath.Base(path)
	n := &node{
		fid:         fid,
		parentFid:   parentFid,
		name:        name,
		size:        int64(size),
		currentPath: path,
		localPath:   localPath,
		isDirty:     true,
		isFolder:    false,
		mtime:       time.Now(),
	}
	fs.storeNode(path, n)
	return n
}

// ==============================================================
// 测试用例
// ==============================================================

// TestUnlinkLocalFileDuringUpload 确认问题：
// local_ 文件在上传队列中被 Unlink 后，uploadWorker 可能竞态完成上传，
// 在服务器上创建"幽灵文件"，且没有 DELETE 任务清理它。
func TestUnlinkLocalFileDuringUpload(t *testing.T) {
	transport := &ghostTestTransport{
		partLatency:    800 * time.Millisecond, // 慢上传
		controlLatency: 5 * time.Millisecond,
	}

	// 保存并覆盖 URL
	oldBase, oldV2, oldAlt := driver.QuarkBaseURL, driver.QuarkV2URL, driver.QuarkV2AltURL
	driver.QuarkBaseURL = "https://drive-pc.quark.cn/1/clouddrive"
	driver.QuarkV2URL = "https://drive-pc.quark.cn/2/clouddrive"
	driver.QuarkV2AltURL = "https://drive-pc.quark.cn/1/clouddrive"
	defer func() {
		driver.QuarkBaseURL = oldBase
		driver.QuarkV2URL = oldV2
		driver.QuarkV2AltURL = oldAlt
	}()

	fs := buildGhostTestFS(t, transport)

	// 创建 dist 目录
	distNode := &node{
		fid: "dist_fid", parentFid: "root_fid", name: "dist",
		currentPath: "/dist", isFolder: true, mtime: time.Now(),
		children: make(map[string]*node),
	}
	fs.storeNode("/dist", distNode)

	// 创建 local_ 文件
	fileNode := createGhostTestFile(t, fs, "/dist/app.js", "local_app.js_race", "dist_fid", 512*1024)

	// 排入上传队列
	fileNode.mu.Lock()
	fileNode.syncQueued = true
	fileNode.mu.Unlock()
	fs.uploadChan <- syncTask{node: fileNode}

	// 等 uploadWorker 开始处理
	time.Sleep(50 * time.Millisecond)

	// 执行 Unlink
	t.Log("Executing Unlink...")
	errc := fs.Unlink("/dist/app.js")
	if errc != 0 {
		t.Fatalf("Unlink failed: %d", errc)
	}
	t.Log("Unlink completed")

	// 等待上传完成（800ms 延迟 + 网络开销）
	time.Sleep(3 * time.Second)

	// 检查结果
	transport.dumpRequests(t.Logf)

	uploadCompleted := transport.hasRequest("/file/upload/finish")
	deleteCalls := transport.countRequests("/file/delete")
	uploadParts := transport.getUploadCount()

	t.Logf("Upload parts: %d, Upload completed: %v, Delete calls: %d",
		uploadParts, uploadCompleted, deleteCalls)

	if uploadCompleted && deleteCalls == 0 {
		// 检查 metadataOpChan 中是否有幽灵文件的 DELETE 任务
		// 注意：Unlink 会先发 LOCAL_CLEANUP 任务，需要排空后检查 DELETE
		foundDelete := false
		for {
			select {
			case task := <-fs.metadataOpChan:
				if task.opType == "DELETE" {
					t.Logf("✅ Ghost file cleanup: DELETE task sent to metadataOpChan for %v", task.fids)
					foundDelete = true
				}
				// LOCAL_CLEANUP from Unlink — skip
			case <-time.After(1 * time.Second):
				if !foundDelete {
					t.Error("❌ BUG: Upload completed after Unlink, no DELETE task to clean up ghost file")
				}
				goto done
			}
			if foundDelete {
				goto done
			}
		}
	done:
	} else if uploadCompleted && deleteCalls > 0 {
		t.Log("✅ Upload completed and DELETE was sent to clean up")
	} else {
		t.Log("Upload did not complete (staging file was deleted before upload could start)")
	}
}

// TestUnlinkUploadedFileSendsDelete 验证已上传文件（真实 FID）被 Unlink 后
// 正确发送 DELETE 任务到 metadataOpChan。
func TestUnlinkUploadedFileSendsDelete(t *testing.T) {
	transport := &ghostTestTransport{
		partLatency:    1 * time.Millisecond,
		controlLatency: 1 * time.Millisecond,
	}

	oldBase, oldV2, oldAlt := driver.QuarkBaseURL, driver.QuarkV2URL, driver.QuarkV2AltURL
	driver.QuarkBaseURL = "https://drive-pc.quark.cn/1/clouddrive"
	driver.QuarkV2URL = "https://drive-pc.quark.cn/2/clouddrive"
	driver.QuarkV2AltURL = "https://drive-pc.quark.cn/1/clouddrive"
	defer func() {
		driver.QuarkBaseURL = oldBase
		driver.QuarkV2URL = oldV2
		driver.QuarkV2AltURL = oldAlt
	}()

	fs := buildGhostTestFS(t, transport)

	distNode := &node{
		fid: "dist_fid", parentFid: "root_fid", name: "dist",
		currentPath: "/dist", isFolder: true, mtime: time.Now(),
		children: make(map[string]*node),
	}
	fs.storeNode("/dist", distNode)

	// 创建文件并手动设置为已上传状态
	fileNode := createGhostTestFile(t, fs, "/dist/app.js", "local_app.js_uploaded", "dist_fid", 100*1024)

	// 先上传完成
	fileNode.mu.Lock()
	fileNode.syncQueued = true
	fileNode.mu.Unlock()
	fs.uploadChan <- syncTask{node: fileNode}
	time.Sleep(2 * time.Second)

	// 确认上传成功
	fileNode.mu.RLock()
	fid := fileNode.fid
	isDirty := fileNode.isDirty
	fileNode.mu.RUnlock()
	t.Logf("After upload: fid=%s, isDirty=%v", fid, isDirty)

	if isDirty || strings.HasPrefix(fid, "local_") {
		t.Fatalf("Upload did not complete: fid=%s, isDirty=%v", fid, isDirty)
	}

	// Unlink
	t.Log("Unlinking uploaded file...")
	errc := fs.Unlink("/dist/app.js")
	if errc != 0 {
		t.Fatalf("Unlink failed: %d", errc)
	}

	// 检查 metadataOpChan 中是否有 DELETE 任务
	select {
	case task := <-fs.metadataOpChan:
		if task.opType != "DELETE" {
			t.Fatalf("Expected DELETE task, got %s", task.opType)
		}
		if len(task.fids) != 1 || task.fids[0] != fid {
			t.Fatalf("DELETE task fids mismatch: expected [%s], got %v", fid, task.fids)
		}
		t.Logf("✅ DELETE task sent for fid=%s", fid)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout: no DELETE task received in metadataOpChan")
	}
}
