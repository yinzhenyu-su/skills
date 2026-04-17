package vfs

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/pkg/xattr"
	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/driver"
)

type e2eConfig struct {
	cookie     string
	password   string
	salt       string
	remotePath string
	mountPoint string
	cacheDir   string
}

type testLogger struct {
	t *testing.T
}

func (l *testLogger) Printf(format string, v ...interface{}) {
	l.t.Logf(format, v...)
}

type perfTestLogger struct {
	t *testing.T
}

func (l *perfTestLogger) Printf(format string, v ...interface{}) {
	if strings.HasPrefix(format, "[FUSE] Write:") || strings.HasPrefix(format, "[FUSE] Read:") {
		return
	}
	l.t.Logf(format, v...)
}

func writePatternFile(path string, totalSize int64, chunk []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	for written := int64(0); written < totalSize; {
		buf := chunk
		remaining := totalSize - written
		if remaining < int64(len(buf)) {
			buf = buf[:remaining]
		}
		n, err := f.Write(buf)
		if err != nil {
			return err
		}
		written += int64(n)
	}

	return f.Sync()
}

func readFileSample(path string, offset int64, size int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := make([]byte, size)
	n, err := f.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return buf[:n], nil
}

var (
	e2eRunIDOnce sync.Once
	e2eRunID     string
)

func getE2ERunID() string {
	e2eRunIDOnce.Do(func() {
		randBuf := make([]byte, 4)
		_, _ = rand.Read(randBuf)
		e2eRunID = fmt.Sprintf("run_%d_%x", time.Now().UnixNano(), randBuf)
	})
	return e2eRunID
}

func loadE2EConfig(t *testing.T) *e2eConfig {
	// 优先从 qrypt 项目根目录加载 .env.local
	_ = godotenv.Load("../../.env.local")
	_ = godotenv.Load("../../.env")
	_ = godotenv.Load(".env.local")
	_ = godotenv.Load(".env")

	config := &e2eConfig{
		cookie:     os.Getenv("QRYPT_TEST_COOKIE"),
		password:   os.Getenv("QRYPT_TEST_PASSWORD"),
		salt:       os.Getenv("QRYPT_TEST_SALT"),
		remotePath: os.Getenv("QRYPT_TEST_REMOTE_PATH"),
		mountPoint: os.Getenv("QRYPT_TEST_MOUNT_POINT"),
		cacheDir:   os.Getenv("QRYPT_TEST_CACHE_DIR"),
	}

	if config.remotePath != "" {
		config.remotePath = filepath.Join(config.remotePath, getE2ERunID())
	}

	if config.cookie == "" || config.password == "" || config.remotePath == "" || config.mountPoint == "" || config.cacheDir == "" {
		t.Skip("Skipping E2E test: QRYPT_TEST_COOKIE, QRYPT_TEST_PASSWORD, QRYPT_TEST_REMOTE_PATH, QRYPT_TEST_MOUNT_POINT, or QRYPT_TEST_CACHE_DIR not set")
	}

	return config
}

func unmount(mountPoint string) {
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("umount", mountPoint)
	} else {
		cmd = exec.Command("fusermount", "-u", mountPoint)
	}
	cmd.Run()
}

func ensureRemotePath(d *driver.QuarkDriver, path string) (string, error) {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	currentFid := "0"

	for _, seg := range segments {
		if seg == "" {
			continue
		}
		fid, err := d.FindChildByName(currentFid, seg)
		if err != nil {
			// Try to create it, with retries for "doloading" transient conflicts
			for createAttempt := 0; createAttempt < 10; createAttempt++ {
				fid, err = d.CreateDir(currentFid, seg)
				if err == nil {
					break
				}
				if strings.Contains(err.Error(), "23008") || strings.Contains(err.Error(), "conflict") {
					// Directory might be in "doloading" transient state — wait and retry
					time.Sleep(time.Duration(createAttempt+1) * 2 * time.Second)
					d.RemoveDirCache(currentFid)
					fid, err = d.FindChildByName(currentFid, seg)
					if err == nil {
						break
					}
					continue
				}
				return "", fmt.Errorf("failed to create remote dir %s: %v", seg, err)
			}
			if err != nil {
				// Final attempt: maybe it appeared in the listing by now
				d.RemoveDirCache(currentFid)
				fid, err = d.FindChildByName(currentFid, seg)
				if err != nil {
					return "", fmt.Errorf("failed to create remote dir %s after retries: %v", seg, err)
				}
			}
		}
		currentFid = fid
	}
	return currentFid, nil
}

func setupQryptFSInternal(t *testing.T, config *e2eConfig, clearCache bool) (*QryptFS, *fuse.FileSystemHost, error) {
	// 设置测试专用的 Logger
	driver.Log = &testLogger{t: t}

	if clearCache {
		unmount(config.mountPoint)
		os.RemoveAll(config.cacheDir)
		os.MkdirAll(config.mountPoint, 0755)
	}

	cipher, err := crypt.NewRcloneCipher(config.password, config.salt)
	if err != nil {
		return nil, nil, fmt.Errorf("cipher init failed: %v", err)
	}

	d := driver.NewQuarkDriver(config.cookie)
	if err := d.Auth(); err != nil {
		return nil, nil, fmt.Errorf("auth failed: %v", err)
	}

	if clearCache {
		d.RemoveDirCache("0")

		// 递归清理远程测试目录中的残留
		rootFid, err := ensureRemotePath(d, config.remotePath)
		if err == nil {
			files, err := d.ListFiles(rootFid)
			if err == nil {
				var fids []string
				for _, f := range files {
					fids = append(fids, f.Fid)
				}
				if len(fids) > 0 {
					d.Delete(fids)
				}
			}
			d.RemoveDirCache(rootFid)
		}
	}

	rootFid, err := ensureRemotePath(d, config.remotePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to ensure remote path: %v", err)
	}

	if clearCache {
		if err := os.MkdirAll(config.cacheDir, 0755); err != nil {
			return nil, nil, fmt.Errorf("failed to create cache dir: %v", err)
		}
	}

	dbPath := fmt.Sprintf("%s/qrypt_test.db", config.cacheDir)
	cm, err := cache.NewCacheManager(config.cacheDir, dbPath, 1024*1024*1024)
	if err != nil {
		return nil, nil, fmt.Errorf("cache init failed: %v", err)
	}

	fs := NewQryptFS(d, cm, rootFid, cipher)
	host := fuse.NewFileSystemHost(fs)
	// 6. 后台挂载
	options := []string{
		"-o", "rw",
		"-o", "nonempty",
	}
	if runtime.GOOS == "darwin" {
		options = append(options,
			"-o", "noappledouble",
			"-o", "volname=QryptTest",
			"-o", "defer_permissions",
			"-o", "local",
		)
	} else {
		options = append(options,
			"-o", "allow_other",
			"-o", "default_permissions",
		)
	}

	go host.Mount(config.mountPoint, options)

	success := false
	for i := 0; i < 20; i++ {
		time.Sleep(500 * time.Millisecond)
		if _, err := os.Stat(config.mountPoint); err == nil {
			success = true
			break
		}
	}
	if !success {
		return nil, nil, fmt.Errorf("mount failed to become ready at %s", config.mountPoint)
	}

	// For standard E2E tests, we cleanup the remote dir on exit.
	// But only for the first mount in a test to avoid deleting it while it's being used by a second mount.
	if clearCache {
		t.Cleanup(func() {
			d := driver.NewQuarkDriver(config.cookie)
			rootFid, err := ensureRemotePath(d, config.remotePath)
			if err == nil {
				_ = d.Delete([]string{rootFid})
			}
		})
	}

	return fs, host, nil
}

func waitForSync(t *testing.T, fs *QryptFS, path string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if v, ok := fs.nodes.Load(path); ok {
			n := v.(*node)
			n.mu.RLock()
			dirty := n.isDirty
			n.mu.RUnlock()
			if !dirty {
				return
			}
		}
		time.Sleep(1 * time.Second)
	}
	t.Fatalf("Timeout waiting for sync: %s", path)
}

func remoteEntryExists(config *e2eConfig, relPath string) (bool, error) {
	cipher, err := crypt.NewRcloneCipher(config.password, config.salt)
	if err != nil {
		return false, err
	}

	d := driver.NewQuarkDriver(config.cookie)
	if err := d.Auth(); err != nil {
		return false, err
	}

	currentFid, err := d.ResolvePath(config.remotePath)
	if err != nil {
		return false, err
	}

	parts := strings.Split(strings.Trim(relPath, "/"), "/")
	for i, part := range parts {
		if part == "" {
			continue
		}

		d.RemoveDirCache(currentFid)
		files, err := d.ListFiles(currentFid)
		if err != nil {
			return false, err
		}

		found := false
		for _, f := range files {
			decName, _ := cipher.DecryptSegment(f.FileName)
			if decName != part {
				continue
			}
			if i == len(parts)-1 {
				return true, nil
			}
			currentFid = f.Fid
			found = true
			break
		}
		if !found {
			return false, nil
		}
	}

	return true, nil
}

func waitForRemoteEntryState(t *testing.T, config *e2eConfig, relPath string, wantExists bool) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		exists, err := remoteEntryExists(config, relPath)
		if err == nil && exists == wantExists {
			return
		}
		time.Sleep(1 * time.Second)
	}
	t.Fatalf("Timeout waiting for remote path state %q => %t", relPath, wantExists)
}

func TestE2E_Lifecycle(t *testing.T) {
	config := loadE2EConfig(t)

	// Pre-cleanup in case of previous failures
	os.RemoveAll(filepath.Join(config.mountPoint, "it_test_dir"))
	time.Sleep(1 * time.Second)

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	testDir := filepath.Join(config.mountPoint, "it_test_dir")
	if err := os.Mkdir(testDir, 0755); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	testFile := filepath.Join(testDir, "it_lifecycle_file.txt")
	content := []byte("Hello, Qrypt E2E Integration Test!")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	waitForSync(t, fs, "/it_test_dir/it_lifecycle_file.txt")

	readContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(content, readContent) {
		t.Errorf("Content mismatch. Expected %q, got %q", string(content), string(readContent))
	}

	newName := filepath.Join(testDir, "it_lifecycle_renamed.txt")
	if err := os.Rename(testFile, newName); err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	host.Unmount()
	unmount(config.mountPoint)
	time.Sleep(3 * time.Second)

	fs2, host2, err := setupQryptFSInternal(t, config, false)
	if err != nil {
		t.Fatalf("Failed to re-setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host2.Unmount()

	newNameRel := filepath.Join(config.mountPoint, "it_test_dir", "it_lifecycle_renamed.txt")
	readContent2, err := os.ReadFile(newNameRel)
	if err != nil {
		t.Fatalf("ReadFile after remount failed: %v", err)
	}
	if !bytes.Equal(content, readContent2) {
		t.Errorf("Content mismatch after remount. Expected %q, got %q", string(content), string(readContent2))
	}

	if err := os.Remove(newNameRel); err != nil {
		t.Fatalf("Remove file failed: %v", err)
	}
	if err := os.Remove(filepath.Join(config.mountPoint, "it_test_dir")); err != nil {
		t.Fatalf("Remove dir failed: %v", err)
	}

	_ = fs2
}

func TestE2E_MoveFileToSubdirectory(t *testing.T) {
	config := loadE2EConfig(t)

	// Pre-cleanup
	os.RemoveAll(filepath.Join(config.mountPoint, "move_test_dir"))
	os.Remove(filepath.Join(config.mountPoint, "move_test_file.txt"))
	time.Sleep(1 * time.Second)

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	// 1. 创建测试文件
	testFile := filepath.Join(config.mountPoint, "move_test_file.txt")
	content := []byte("Hello, Move Test!")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	waitForSync(t, fs, "/move_test_file.txt")
	t.Log("File created and synced")

	// 2. 创建子目录
	subDir := filepath.Join(config.mountPoint, "move_test_dir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}
	time.Sleep(2 * time.Second) // 等待目录创建同步
	t.Log("Subdirectory created")

	// 3. 移动文件到子目录
	movedFile := filepath.Join(subDir, "move_test_file.txt")
	if err := os.Rename(testFile, movedFile); err != nil {
		t.Fatalf("Move to subdirectory failed: %v", err)
	}
	waitForSync(t, fs, "/move_test_dir/move_test_file.txt")
	t.Log("File moved to subdirectory")

	// 4. 验证文件内容
	readContent, err := os.ReadFile(movedFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(content, readContent) {
		t.Errorf("Content mismatch after move. Expected %q, got %q", string(content), string(readContent))
	}

	// 5. 移动文件回根目录
	backToFile := filepath.Join(config.mountPoint, "move_test_file.txt")
	if err := os.Rename(movedFile, backToFile); err != nil {
		t.Fatalf("Move back to root failed: %v", err)
	}
	waitForSync(t, fs, "/move_test_file.txt")
	t.Log("File moved back to root")

	// 6. 验证文件内容
	readContent2, err := os.ReadFile(backToFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(content, readContent2) {
		t.Errorf("Content mismatch after move back. Expected %q, got %q", string(content), string(readContent2))
	}

	// 7. 清理
	if err := os.Remove(backToFile); err != nil {
		t.Fatalf("Remove file failed: %v", err)
	}
	if err := os.Remove(subDir); err != nil {
		t.Fatalf("Remove dir failed: %v", err)
	}

	_ = fs
}

func TestE2E_CreateAndSync(t *testing.T) {
	config := loadE2EConfig(t)

	const fileName = "create_sync.txt"
	testFile := filepath.Join(config.mountPoint, fileName)
	_ = os.Remove(testFile)

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	// 1. Create file and write content
	content := []byte("hello, this is a test file content")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	waitForSync(t, fs, "/"+fileName)

	// 2. Verify content is correct locally
	readContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(content, readContent) {
		t.Fatalf("Content mismatch. Expected %q, got %q", string(content), string(readContent))
	}

	// 3. Remount and verify content persisted to remote
	host.Unmount()
	unmount(config.mountPoint)
	time.Sleep(3 * time.Second)

	_, host2, err := setupQryptFSInternal(t, config, false)
	if err != nil {
		t.Fatalf("Failed to re-setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host2.Unmount()

	// Read from remote via remounted filesystem
	readContent2, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile after remount failed: %v", err)
	}
	if !bytes.Equal(content, readContent2) {
		t.Fatalf("Content mismatch after remount. Expected %q, got %q", string(content), string(readContent2))
	}

	// Cleanup
	if err := os.Remove(testFile); err != nil {
		t.Fatalf("Remove file failed: %v", err)
	}
}

func TestE2E_DeleteSyncedFileRemovesRemote(t *testing.T) {
	config := loadE2EConfig(t)

	const fileName = "delete_after_sync.txt"
	testFile := filepath.Join(config.mountPoint, fileName)
	_ = os.Remove(testFile)

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	content := []byte("delete-after-sync")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	waitForSync(t, fs, "/"+fileName)
	waitForRemoteEntryState(t, config, fileName, true)

	if err := os.Remove(testFile); err != nil {
		t.Fatalf("Remove file failed: %v", err)
	}

	waitForRemoteEntryState(t, config, fileName, false)
}

func TestE2E_Upload5MBFile(t *testing.T) {
	config := loadE2EConfig(t)

	const fileName = "upload_5mb.bin"
	testFile := filepath.Join(config.mountPoint, fileName)
	_ = os.Remove(testFile)

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	content := make([]byte, 5*1024*1024)
	if _, err := rand.Read(content); err != nil {
		t.Fatalf("Failed to generate test content: %v", err)
	}

	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	waitForSync(t, fs, "/"+fileName)

	readContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(content, readContent) {
		t.Fatalf("Content mismatch before remount")
	}

	host.Unmount()
	unmount(config.mountPoint)
	time.Sleep(3 * time.Second)

	_, host2, err := setupQryptFSInternal(t, config, false)
	if err != nil {
		t.Fatalf("Failed to re-setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host2.Unmount()

	remountedFile := filepath.Join(config.mountPoint, fileName)
	readContentAfterRemount, err := os.ReadFile(remountedFile)
	if err != nil {
		t.Fatalf("ReadFile after remount failed: %v", err)
	}
	if !bytes.Equal(content, readContentAfterRemount) {
		t.Fatalf("Content mismatch after remount")
	}

	if err := os.Remove(remountedFile); err != nil {
		t.Fatalf("Remove file failed: %v", err)
	}
}

func TestE2E_UploadPerf200MB(t *testing.T) {
	config := loadE2EConfig(t)

	const (
		fileName   = "upload_perf_200mb.bin"
		totalSize  = 200 * 1024 * 1024
		chunkSize  = 1 * 1024 * 1024
		sampleSize = 4 * 1024
	)
	testFile := filepath.Join(config.mountPoint, fileName)
	_ = os.Remove(testFile)

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()
	driver.Log = &perfTestLogger{t: t}

	recorder := newSyncPerfRecorder()
	fs.syncObserver = recorder

	chunk := make([]byte, chunkSize)
	if _, err := rand.Read(chunk); err != nil {
		t.Fatalf("Failed to generate test chunk: %v", err)
	}
	expectedStart := append([]byte(nil), chunk[:sampleSize]...)
	expectedEnd := append([]byte(nil), chunk[chunkSize-sampleSize:]...)

	writeStartedAt := time.Now()
	if err := writePatternFile(testFile, totalSize, chunk); err != nil {
		t.Fatalf("Write pattern file failed: %v", err)
	}

	obs := recorder.Wait(t, 10*time.Minute)
	if obs.Err != nil {
		t.Fatalf("Background sync failed: %v", obs.Err)
	}

	endToEnd := obs.FinishedAt.Sub(writeStartedAt)
	throughputMiBS := float64(totalSize) / endToEnd.Seconds() / 1024 / 1024
	queueDelay := time.Duration(0)
	if !obs.StartedAt.IsZero() {
		queueDelay = obs.StartedAt.Sub(writeStartedAt)
	}

	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Size() != totalSize {
		t.Fatalf("Size mismatch after perf upload: expected %d, got %d", totalSize, info.Size())
	}

	startSample, err := readFileSample(testFile, 0, sampleSize)
	if err != nil {
		t.Fatalf("Read start sample failed: %v", err)
	}
	if !bytes.Equal(expectedStart, startSample) {
		t.Fatalf("Start sample mismatch after perf upload")
	}

	endSample, err := readFileSample(testFile, totalSize-sampleSize, sampleSize)
	if err != nil {
		t.Fatalf("Read end sample failed: %v", err)
	}
	if !bytes.Equal(expectedEnd, endSample) {
		t.Fatalf("End sample mismatch after perf upload")
	}

	t.Logf("e2e upload 200MiB: queue=%s sync=%s total=%s throughput=%.2f MiB/s pre=%s upload=%s update_hash=%s commit=%s finish=%s parts=%d uploaded=%d",
		queueDelay,
		obs.Snapshot.TotalDuration,
		endToEnd,
		throughputMiBS,
		obs.Snapshot.PreDuration,
		obs.Snapshot.UploadPartDuration,
		obs.Snapshot.UpdateHashDuration,
		obs.Snapshot.CommitDuration,
		obs.Snapshot.FinishDuration,
		obs.Snapshot.PartCount,
		obs.Snapshot.UploadedBytes,
	)

	if err := os.Remove(testFile); err != nil {
		t.Fatalf("Remove file failed: %v", err)
	}
}

func TestE2E_XAttr(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("xattr test only supported on darwin/linux")
	}

	config := loadE2EConfig(t)
	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	testFile := filepath.Join(config.mountPoint, "xattr_test.txt")
	if err := os.WriteFile(testFile, []byte("xattr test content"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	waitForSync(t, fs, "/xattr_test.txt")

	attrName := "user.test_attr"
	if runtime.GOOS == "darwin" {
		attrName = "test_attr"
	}
	attrValue := []byte("test_value")

	if err := xattr.Set(testFile, attrName, attrValue); err != nil {
		t.Errorf("xattr.Set failed: %v", err)
	}

	gotValue, err := xattr.Get(testFile, attrName)
	if err != nil {
		t.Logf("xattr.Get returned error (expected if not persisted): %v", err)
	} else if !bytes.Equal(attrValue, gotValue) {
		t.Errorf("xattr value mismatch. Expected %q, got %q", string(attrValue), string(gotValue))
	}

	if err := os.Remove(testFile); err != nil {
		t.Fatalf("Remove file failed: %v", err)
	}
}

func TestE2E_ConcurrentStress(t *testing.T) {
	config := loadE2EConfig(t)
	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	stressDir := filepath.Join(config.mountPoint, "stress_test")
	if err := os.Mkdir(stressDir, 0755); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	numFiles := 10
	var wg sync.WaitGroup
	for i := 0; i < numFiles; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			fileName := fmt.Sprintf("file_%d.txt", idx)
			filePath := filepath.Join(stressDir, fileName)
			content := []byte(fmt.Sprintf("content for file %d", idx))
			if err := os.WriteFile(filePath, content, 0644); err != nil {
				t.Errorf("Failed to write %s: %v", fileName, err)
				return
			}
			waitForSync(t, fs, "/stress_test/"+fileName)
		}(i)
	}
	wg.Wait()

	// Verify all files
	entries, err := os.ReadDir(stressDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != numFiles {
		t.Errorf("Expected %d files, found %d", numFiles, len(entries))
	}

	// Cleanup
	if err := os.RemoveAll(stressDir); err != nil {
		t.Fatalf("RemoveAll failed: %v", err)
	}
}

func TestE2E_ErrorHandling(t *testing.T) {
	config := loadE2EConfig(t)
	_, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	// Test accessing non-existent file
	noFile := filepath.Join(config.mountPoint, "non_existent.txt")
	if _, err := os.Stat(noFile); !os.IsNotExist(err) {
		t.Errorf("Expected NotExist error for %s, got %v", noFile, err)
	}

	// Test creating directory in non-existent parent
	badDir := filepath.Join(config.mountPoint, "ghost/new_dir")
	if err := os.Mkdir(badDir, 0755); err == nil {
		t.Errorf("Expected error when creating directory in non-existent parent")
	}
}

func TestE2E_LargeDirectoryLoadPerformance(t *testing.T) {
	config := loadE2EConfig(t)

	// Pre-cleanup
	os.RemoveAll(filepath.Join(config.mountPoint, "perf_large_dir"))
	time.Sleep(1 * time.Second)

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}

	testDir := filepath.Join(config.mountPoint, "perf_large_dir")
	if err := os.Mkdir(testDir, 0755); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	numFiles := 55
	var wg sync.WaitGroup
	for i := 0; i < numFiles; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			fileName := fmt.Sprintf("perf_file_%d.txt", idx)
			filePath := filepath.Join(testDir, fileName)
			content := []byte(fmt.Sprintf("Content of file %d", idx))
			if err := os.WriteFile(filePath, content, 0644); err != nil {
				t.Errorf("WriteFile failed for %s: %v", fileName, err)
			}
		}(i)
	}
	wg.Wait()

	// Wait for all files to sync
	for i := 0; i < numFiles; i++ {
		vfsPath := fmt.Sprintf("/perf_large_dir/perf_file_%d.txt", i)
		waitForSync(t, fs, vfsPath)
	}

	// Unmount to simulate cold start
	host.Unmount()
	unmount(config.mountPoint)
	time.Sleep(3 * time.Second)

	// Remount
	fs2, host2, err := setupQryptFSInternal(t, config, false)
	if err != nil {
		t.Fatalf("Failed to re-setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host2.Unmount()

	_ = fs2

	// Measure loading speed
	start := time.Now()
	entries, err := os.ReadDir(filepath.Join(config.mountPoint, "perf_large_dir"))
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}

	if len(entries) != numFiles {
		t.Errorf("Expected %d files, got %d", numFiles, len(entries))
	}

	t.Logf("Loaded %d files in %v", len(entries), elapsed)

	// We expect the optimized load to take less than 5 seconds for 55 files (including network request + decryption)
	if elapsed > 5*time.Second {
		t.Errorf("Loading speed too slow! Took %v", elapsed)
	}
}

func TestE2E_CopyConflict(t *testing.T) {
	config := loadE2EConfig(t)

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	// 1. Create directory A
	dirA := filepath.Join(config.mountPoint, "A")
	if err := os.Mkdir(dirA, 0755); err != nil {
		t.Fatalf("Mkdir A failed: %v", err)
	}

	// 2. Create file /A/file.txt
	fileA := filepath.Join(dirA, "file.txt")
	contentA := []byte("content from A")
	if err := os.WriteFile(fileA, contentA, 0644); err != nil {
		t.Fatalf("WriteFile /A/file.txt failed: %v", err)
	}
	waitForSync(t, fs, "/A/file.txt")

	// 3. Create /file.txt (existing file to be overwritten)
	fileRoot := filepath.Join(config.mountPoint, "file.txt")
	contentRoot := []byte("original root content")
	if err := os.WriteFile(fileRoot, contentRoot, 0644); err != nil {
		t.Fatalf("WriteFile /file.txt failed: %v", err)
	}
	waitForSync(t, fs, "/file.txt")
	waitForRemoteEntryState(t, config, "file.txt", true)

	// 4. Copy /A/file.txt to /file.txt (Overwrite)
	// We use shell 'cp' to simulate real OS behavior
	cmd := exec.Command("cp", fileA, fileRoot)
	if err := cmd.Run(); err != nil {
		t.Fatalf("cp failed: %v", err)
	}
	
	waitForSync(t, fs, "/file.txt")
	
	// Verify content locally
	gotContent, err := os.ReadFile(fileRoot)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(gotContent, contentA) {
		t.Errorf("Content mismatch. Expected %q, got %q", string(contentA), string(gotContent))
	}

	// 5. Check remote for duplicates
	cipher, err := crypt.NewRcloneCipher(config.password, config.salt)
	if err != nil {
		t.Fatalf("cipher init failed: %v", err)
	}

	d := driver.NewQuarkDriver(config.cookie)
	if err := d.Auth(); err != nil {
		t.Fatalf("auth failed: %v", err)
	}

	rootFid, err := d.ResolvePath(config.remotePath)
	if err != nil {
		t.Fatalf("resolve remote path failed: %v", err)
	}

	// Give it some time for remote to reflect changes if there's any eventual consistency
	time.Sleep(2 * time.Second)
	d.RemoveDirCache(rootFid)
	files, err := d.ListFiles(rootFid)
	if err != nil {
		t.Fatalf("list files failed: %v", err)
	}

	foundFileTxt := false
	for _, f := range files {
		decName, _ := cipher.DecryptSegment(f.FileName)
		if decName == "file.txt" {
			foundFileTxt = true
		}
		if strings.Contains(decName, "file.txt") && strings.Contains(decName, "(") {
			t.Errorf("Found unexpected remote file (potential duplicate): %s", decName)
		}
	}
	
	if !foundFileTxt {
		t.Errorf("file.txt not found on remote")
	}
}

// TestE2E_CrossDirectoryRename tests moving a file across directories
// (the macOS Finder cut-paste scenario). This reproduces the bug where
// Move API was missing the required current_dir_fid parameter.
func TestE2E_CrossDirectoryRename(t *testing.T) {
	config := loadE2EConfig(t)

	// Pre-cleanup
	os.RemoveAll(filepath.Join(config.mountPoint, "e2e_rename_src"))
	time.Sleep(1 * time.Second)

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	// 1. Create source directory
	srcDir := filepath.Join(config.mountPoint, "e2e_rename_src")
	if err := os.Mkdir(srcDir, 0755); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}
	time.Sleep(2 * time.Second) // wait for dir sync

	// 2. Create file in subdirectory and wait for sync
	srcFile := filepath.Join(srcDir, "move_me.txt")
	content := []byte("cross-directory rename test content")
	if err := os.WriteFile(srcFile, content, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	waitForSync(t, fs, "/e2e_rename_src/move_me.txt")

	// 3. Move file from /e2e_rename_src/ to / (cross-directory rename)
	dstFile := filepath.Join(config.mountPoint, "move_me.txt")
	if err := os.Rename(srcFile, dstFile); err != nil {
		t.Fatalf("Cross-directory Rename failed: %v", err)
	}

	// 4. Verify file accessible at new path
	readBack, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("ReadFile at new path failed: %v", err)
	}
	if !bytes.Equal(content, readBack) {
		t.Errorf("Content mismatch. Expected %q, got %q", string(content), string(readBack))
	}

	// 5. Verify file gone from old path
	if _, err := os.Stat(srcFile); !os.IsNotExist(err) {
		t.Errorf("File should not exist at old path %s", srcFile)
	}

	// 6. Cleanup
	os.Remove(dstFile)
	os.Remove(srcDir)
}

// TestE2E_NoDuplicateAfterUpload verifies that after uploading a file,
// there's only one copy on the server (no empty placeholder or (1) suffix).
func TestE2E_NoDuplicateAfterUpload(t *testing.T) {
	config := loadE2EConfig(t)

	const fileName = "no_dup_test.txt"
	os.Remove(filepath.Join(config.mountPoint, fileName))
	time.Sleep(1 * time.Second)

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	// 1. Write and sync
	content := []byte("no duplicate test content " + fmt.Sprint(time.Now().UnixNano()))
	testFile := filepath.Join(config.mountPoint, fileName)
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	waitForSync(t, fs, "/"+fileName)

	// 2. Wait for remote to stabilize
	time.Sleep(3 * time.Second)

	// 3. Check remote: count files with matching encrypted name
	cipher, err := crypt.NewRcloneCipher(config.password, config.salt)
	if err != nil {
		t.Fatalf("cipher init failed: %v", err)
	}
	d := driver.NewQuarkDriver(config.cookie)
	if err := d.Auth(); err != nil {
		t.Fatalf("auth failed: %v", err)
	}
	rootFid, err := d.ResolvePath(config.remotePath)
	if err != nil {
		t.Fatalf("resolve path failed: %v", err)
	}
	d.RemoveDirCache(rootFid)
	files, err := d.ListFiles(rootFid)
	if err != nil {
		t.Fatalf("list files failed: %v", err)
	}

	encName := cipher.EncryptSegment(fileName)
	matchCount := 0
	for _, f := range files {
		if f.FileName == encName {
			matchCount++
			if f.Int64Size() == 0 {
				t.Errorf("Found empty placeholder file (fid=%s) — UploadFinish not called after dedup?", f.Fid)
			}
		}
		// Also check for (1) suffixed duplicates
		decName, _ := cipher.DecryptSegment(f.FileName)
		if strings.Contains(decName, fileName[:len(fileName)-4]) && decName != fileName {
			t.Errorf("Found duplicate file on server: %s", decName)
		}
	}
	if matchCount == 0 {
		t.Error("Uploaded file not found on server")
	}
	if matchCount > 1 {
		t.Errorf("Found %d files with same encrypted name (expected 1) — possible placeholder leak", matchCount)
	}

	// 4. Cleanup
	os.Remove(testFile)
}
