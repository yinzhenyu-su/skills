package vfs

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/driver"
	"github.com/yinzhenyu/skills/qrypt/internal/staging"
	uploadpkg "github.com/yinzhenyu/skills/qrypt/internal/upload"
)

// TestSync_DeleteExistingBeforeUploadPre 验证修复：
// deleteExistingFileByName 必须在 UploadPre 之前执行，
// 否则 UploadPre 创建的 placeholder 会与旧文件产生同名冲突导致 (1)。
func TestSync_DeleteExistingBeforeUploadPre(t *testing.T) {
	transport := &ghostTestTransport{
		partLatency:    1 * time.Millisecond,
		controlLatency: 1 * time.Millisecond,
	}

	d := driver.NewQuarkDriver("mock_cookie")
	d.SetClient(&http.Client{Transport: transport})

	cipher, err := crypt.NewRcloneCipher("test-password", "")
	if err != nil {
		t.Fatalf("cipher init failed: %v", err)
	}

	cacheDir := t.TempDir()
	cm, err := cache.NewCacheManager(cacheDir, filepath.Join(cacheDir, "test.db"), 1<<30)
	if err != nil {
		t.Fatalf("cache init failed: %v", err)
	}
	defer cm.Close()

	stagingStore, err := staging.NewStore(filepath.Join(cacheDir, "staging"))
	if err != nil {
		t.Fatalf("staging init failed: %v", err)
	}

	m := uploadpkg.NewManager(d, cipher, stagingStore)

	// 创建 staging 文件并写入数据
	localPath, err := stagingStore.Create("local_test_file")
	if err != nil {
		t.Fatalf("staging create failed: %v", err)
	}
	testData := make([]byte, 1024)
	for i := range testData {
		testData[i] = byte(i % 251)
	}
	if _, err := stagingStore.WriteAt(localPath, testData, 0); err != nil {
		t.Fatalf("staging write failed: %v", err)
	}

	// 执行 Sync
	_, err = m.Sync(uploadpkg.SyncRequest{
		Path:      "/test/file.bin",
		Name:      "file.bin",
		ParentFid: "parent_fid",
		LocalPath: localPath,
		PlainSize: 1024,
	})
	if err != nil {
		t.Fatalf("Sync failed: %v", err)
	}

	// 验证请求顺序：ListFiles (deleteExistingFileByName) 必须在 UploadPre 之前
	requests := transport.getRequests()
	listFilesIdx := -1
	uploadPreIdx := -1
	for i, r := range requests {
		if strings.Contains(r.Path, "/file/sort") {
			listFilesIdx = i
		}
		if strings.Contains(r.Path, "/file/upload/pre") {
			uploadPreIdx = i
		}
	}

	if listFilesIdx == -1 {
		t.Fatal("ListFiles (file/sort) was never called — deleteExistingFileByName not invoked")
	}
	if uploadPreIdx == -1 {
		t.Fatal("UploadPre was never called")
	}
	if listFilesIdx > uploadPreIdx {
		t.Errorf("deleteExistingFileByName (ListFiles at [%d]) was called AFTER UploadPre (at [%d]), should be BEFORE",
			listFilesIdx, uploadPreIdx)
	}

	t.Logf("✅ Request order correct: ListFiles[%d] → UploadPre[%d]", listFilesIdx, uploadPreIdx)
}

// TestSyncFile_UploadsEmptyFile 验证：
// touch 创建的空文件（size=0, staging=0）应正常上传。
func TestSyncFile_UploadsEmptyFile(t *testing.T) {
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

	// 创建 staging 文件但不写入数据（模拟 touch 空文件）
	localPath, err := fs.staging.Create("local_touch_test")
	if err != nil {
		t.Fatalf("staging create failed: %v", err)
	}

	fileNode := &node{
		fid: "local_touch_test", parentFid: "dist_fid", name: "empty.txt",
		size: 0, currentPath: "/dist/empty.txt", localPath: localPath,
		isDirty: true, isFolder: false, mtime: time.Now(),
	}
	// writeInFlight = 0（没有 Write 在进行中）
	fs.storeNode("/dist/empty.txt", fileNode)

	// 调用 syncFile
	err = fs.syncFile("/dist/empty.txt", fileNode)
	if err != nil {
		t.Fatalf("syncFile returned error: %v", err)
	}

	// 验证：应有上传请求（空文件是合法的）
	if !transport.hasRequest("/file/upload/pre") {
		t.Error("syncFile should have uploaded empty file (touch), but UploadPre was not called")
	}

	// 验证：文件应标记为已同步
	fileNode.mu.RLock()
	isDirty := fileNode.isDirty
	fileNode.mu.RUnlock()
	if isDirty {
		t.Error("file should not be dirty after successful sync")
	}

	t.Log("✅ syncFile correctly uploaded empty file when no writeInFlight (touch)")
}

// TestSyncFile_UploadsWhenStagingHasData 验证：
// 当 staging 有数据时，syncFile 正常上传（不会被空文件保护误拦截）。
func TestSyncFile_UploadsWhenStagingHasData(t *testing.T) {
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

	// 创建 staging 文件并写入数据
	localPath, err := fs.staging.Create("local_data_test")
	if err != nil {
		t.Fatalf("staging create failed: %v", err)
	}
	testData := make([]byte, 4096)
	for i := range testData {
		testData[i] = byte(i % 251)
	}
	if _, err := fs.staging.WriteAt(localPath, testData, 0); err != nil {
		t.Fatalf("staging write failed: %v", err)
	}

	fileNode := &node{
		fid:         "local_data_test",
		parentFid:   "dist_fid",
		name:        "data.txt",
		size:        4096,
		currentPath: "/dist/data.txt",
		localPath:   localPath,
		isDirty:     true,
		isFolder:    false,
		mtime:       time.Now(),
	}
	fs.storeNode("/dist/data.txt", fileNode)

	// 调用 syncFile
	err = fs.syncFile("/dist/data.txt", fileNode)
	if err != nil {
		t.Fatalf("syncFile returned error: %v", err)
	}

	// 验证：应该有上传请求
	if !transport.hasRequest("/file/upload/pre") {
		t.Error("syncFile should have called UploadPre for non-empty staging file")
	}
	if !transport.hasRequest("/file/upload/finish") {
		t.Error("syncFile should have called UploadFinish for non-empty staging file")
	}

	// 验证：文件应标记为已同步
	fileNode.mu.RLock()
	isDirty := fileNode.isDirty
	fid := fileNode.fid
	fileNode.mu.RUnlock()

	if isDirty {
		t.Error("file should not be dirty after successful sync")
	}
	if strings.HasPrefix(fid, "local_") {
		t.Errorf("fid should be updated to server fid, still local_: %s", fid)
	}

	t.Logf("✅ syncFile correctly uploaded non-empty file: fid=%s", fid)
}
