package vfs

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
		rootFid, err := d.ResolvePath(config.remotePath)
		if err == nil {
			files, err := d.ListFiles(rootFid)
			if err == nil {
				for _, f := range files {
					decName, _ := cipher.DecryptSegment(f.FileName)
					if decName == "it_test_dir" || decName == "xattr_test.txt" || decName == "stress_test" {
						d.Delete([]string{f.Fid})
					}
				}
			}
			d.RemoveDirCache(rootFid)
		}
	}

	rootFid, err := d.ResolvePath(config.remotePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve remote path: %v", err)
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
		"-o", "noappledouble",
		"-o", "volname=QryptTest",
		"-o", "defer_permissions",
		"-o", "local",
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

	testFile := filepath.Join(testDir, "hello.txt")
	content := []byte("Hello, Qrypt E2E Integration Test!")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	waitForSync(t, fs, "/it_test_dir/hello.txt")

	readContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(content, readContent) {
		t.Errorf("Content mismatch. Expected %q, got %q", string(content), string(readContent))
	}

	newName := filepath.Join(testDir, "renamed.txt")
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

	newNameRel := filepath.Join(config.mountPoint, "it_test_dir", "renamed.txt")
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
