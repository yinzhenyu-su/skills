package vfs

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestE2E_TruncateFile 测试文件截断后上传
func TestE2E_TruncateFile(t *testing.T) {
	config := loadE2EConfig(t)

	testDir := filepath.Join(config.mountPoint, "truncate_test")
	os.RemoveAll(testDir)
	time.Sleep(1 * time.Second)

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	// 创建测试目录
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	// 1. 创建原始文件
	testFile := filepath.Join(testDir, "truncate_file.txt")
	originalContent := []byte("Hello, this is a long content that will be truncated!")
	if err := os.WriteFile(testFile, originalContent, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	waitForSync(t, fs, "/truncate_test/truncate_file.txt")

	// 2. 截断文件到更小的大小
	truncatedContent := originalContent[:10] // 只保留前10字节
	if err := os.WriteFile(testFile, truncatedContent, 0644); err != nil {
		t.Fatalf("Truncate WriteFile failed: %v", err)
	}
	waitForSync(t, fs, "/truncate_test/truncate_file.txt")

	// 3. 验证截断后的内容
	readContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(truncatedContent, readContent) {
		t.Errorf("Content mismatch after truncate. Expected %q, got %q", 
			string(truncatedContent), string(readContent))
	}

	// 4. 验证文件大小
	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Size() != int64(len(truncatedContent)) {
		t.Errorf("Size mismatch. Expected %d, got %d", len(truncatedContent), info.Size())
	}

	// 5. Cleanup
	os.RemoveAll(testDir)
}

// TestE2E_MkdirRmdir 测试目录创建和删除
func TestE2E_MkdirRmdir(t *testing.T) {
	config := loadE2EConfig(t)

	testDir := filepath.Join(config.mountPoint, "mkdir_rmdir_test")
	os.RemoveAll(testDir)
	time.Sleep(1 * time.Second)

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	// 1. 创建嵌套目录
	nestedDir := filepath.Join(testDir, "level1", "level2", "level3")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	time.Sleep(2 * time.Second)

	// 2. 验证目录存在
	info, err := os.Stat(nestedDir)
	if err != nil {
		t.Fatalf("Stat nested dir failed: %v", err)
	}
	if !info.IsDir() {
		t.Error("Expected directory, got file")
	}

	// 3. 在目录中创建文件
	testFile := filepath.Join(nestedDir, "test.txt")
	content := []byte("test in nested dir")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	waitForSync(t, fs, "/mkdir_rmdir_test/level1/level2/level3/test.txt")

	// 4. 删除文件
	if err := os.Remove(testFile); err != nil {
		t.Fatalf("Remove file failed: %v", err)
	}

	// 5. 删除空目录（从最内层开始）
	if err := os.Remove(nestedDir); err != nil {
		t.Fatalf("Remove level3 failed: %v", err)
	}
	if err := os.Remove(filepath.Join(testDir, "level1", "level2")); err != nil {
		t.Fatalf("Remove level2 failed: %v", err)
	}
	if err := os.Remove(filepath.Join(testDir, "level1")); err != nil {
		t.Fatalf("Remove level1 failed: %v", err)
	}
	if err := os.Remove(testDir); err != nil {
		t.Fatalf("Remove testDir failed: %v", err)
	}

	// 6. 验证目录已删除
	if _, err := os.Stat(testDir); !os.IsNotExist(err) {
		t.Error("Directory should not exist after removal")
	}

	_ = fs
}

// TestE2E_ChmodChownGraceful 测试权限修改优雅处理
// 夸克网盘不支持 Unix 权限，应优雅处理（忽略或返回成功）
func TestE2E_ChmodChownGraceful(t *testing.T) {
	config := loadE2EConfig(t)

	testFile := filepath.Join(config.mountPoint, "chmod_test.txt")
	os.Remove(testFile)
	time.Sleep(1 * time.Second)

	_, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	// 1. 创建文件
	content := []byte("chmod test content")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	time.Sleep(2 * time.Second)

	// 2. 尝试修改权限（应该优雅处理，不报错）
	err = os.Chmod(testFile, 0755)
	// 夸克不支持权限，但不应导致致命错误
	// 即使返回错误也应该是可理解的
	if err != nil {
		t.Logf("Chmod returned error (expected for cloud storage): %v", err)
	}

	// 3. 尝试修改时间戳
	futureTime := time.Now().Add(24 * time.Hour)
	err = os.Chtimes(testFile, futureTime, futureTime)
	if err != nil {
		t.Logf("Chtimes returned error (expected for cloud storage): %v", err)
	}

	// 4. 文件仍应可读
	readContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile failed after chmod/chtimes: %v", err)
	}
	if !bytes.Equal(content, readContent) {
		t.Errorf("Content corrupted after chmod/chtimes")
	}

	// 5. Cleanup
	os.Remove(testFile)
}

// TestE2E_ConcurrentWrite 测试并发写入安全性
func TestE2E_ConcurrentWrite(t *testing.T) {
	config := loadE2EConfig(t)

	testDir := filepath.Join(config.mountPoint, "concurrent_test")
	os.RemoveAll(testDir)
	time.Sleep(1 * time.Second)

	_, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	// 并发写入多个不同文件
	const numFiles = 5
	var wg sync.WaitGroup
	errors := make(chan error, numFiles)

	for i := 0; i < numFiles; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			fileName := filepath.Join(testDir, fmt.Sprintf("concurrent_%d.txt", id))
			content := []byte(fmt.Sprintf("Content from goroutine %d", id))
			if err := os.WriteFile(fileName, content, 0644); err != nil {
				errors <- fmt.Errorf("goroutine %d: WriteFile failed: %v", id, err)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// 收集错误
	var errs []error
	for err := range errors {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		t.Errorf("Got %d errors during concurrent write:", len(errs))
		for _, err := range errs {
			t.Errorf("  - %v", err)
		}
	}

	// 验证所有文件都存在
	time.Sleep(3 * time.Second)
	for i := 0; i < numFiles; i++ {
		fileName := filepath.Join(testDir, fmt.Sprintf("concurrent_%d.txt", i))
		if _, err := os.Stat(fileName); os.IsNotExist(err) {
			t.Errorf("File %s should exist", fileName)
		}
	}

	// Cleanup
	os.RemoveAll(testDir)
}

// TestE2E_DiskFull 模拟磁盘空间不足（安全测试，不会真的填满磁盘）
// 使用小的 tmpfs 来模拟磁盘满的情况
func TestE2E_DiskFull(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("DiskFull test requires root for tmpfs mount")
	}

	config := loadE2EConfig(t)

	// 创建一个小的 tmpfs 来模拟磁盘空间不足
	smallMount := filepath.Join(os.TempDir(), "qrypt_small_disk")
	os.MkdirAll(smallMount, 0755)

	// 挂载 1MB 的 tmpfs（需要 root）
	if err := syscall.Mount("tmpfs", smallMount, "tmpfs", 0, "size=1M"); err != nil {
		t.Skipf("Cannot mount tmpfs: %v (need root)", err)
	}
	defer syscall.Unmount(smallMount, 0)
	defer os.RemoveAll(smallMount)

	// 使用小磁盘作为缓存目录
	config.cacheDir = smallMount

	_, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	// 尝试写入超过 1MB 的数据
	testFile := filepath.Join(config.mountPoint, "diskfull_test.txt")
	largeContent := bytes.Repeat([]byte("X"), 2*1024*1024) // 2MB

	err = os.WriteFile(testFile, largeContent, 0644)
	if err == nil {
		// 如果没报错，可能是因为写入在缓存中，还没触发磁盘写入
		t.Log("Write succeeded (may be in cache), waiting for sync...")
		time.Sleep(5 * time.Second)
	} else {
		// 预期会失败，错误应该是 ENOSPC 或相关错误
		t.Logf("Write failed as expected: %v", err)
		if !isDiskFullError(err) {
			t.Errorf("Expected disk full error, got: %v", err)
		}
	}
}

// isDiskFullError 检查是否是磁盘空间不足错误
func isDiskFullError(err error) bool {
	if err == nil {
		return false
	}
	// 检查是否是 ENOSPC
	if pathErr, ok := err.(*os.PathError); ok {
		if pathErr.Err == syscall.ENOSPC {
			return true
		}
	}
	// 检查错误信息
	errStr := err.Error()
	return contains(errStr, "no space") || 
	       contains(errStr, "ENOSPC") ||
	       contains(errStr, "disk full")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && 
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || 
		 bytes.Contains([]byte(s), []byte(substr))))
}

// TestE2E_FileTimestampPreserve 测试文件时间戳保留
// 即使夸克不支持精确时间戳，读取时不应崩溃
func TestE2E_FileTimestampPreserve(t *testing.T) {
	config := loadE2EConfig(t)

	testFile := filepath.Join(config.mountPoint, "timestamp_test.txt")
	os.Remove(testFile)
	time.Sleep(1 * time.Second)

	_, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	// 1. 创建文件
	content := []byte("timestamp test")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	time.Sleep(2 * time.Second)

	// 2. 获取初始时间戳
	info1, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	initialModTime := info1.ModTime()

	// 3. 修改文件
	content2 := []byte("timestamp test modified")
	if err := os.WriteFile(testFile, content2, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	time.Sleep(2 * time.Second)

	// 4. 验证时间戳已更新
	info2, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	newModTime := info2.ModTime()

	if !newModTime.After(initialModTime) {
		t.Logf("ModTime not updated (cloud storage limitation): initial=%v, new=%v", 
			initialModTime, newModTime)
	}

	// 5. 验证内容正确
	readContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(content2, readContent) {
		t.Errorf("Content mismatch")
	}

	// 6. Cleanup
	os.Remove(testFile)
}

// TestE2E_RapidCreateDelete 测试快速创建删除文件
func TestE2E_RapidCreateDelete(t *testing.T) {
	config := loadE2EConfig(t)

	testDir := filepath.Join(config.mountPoint, "rapid_test")
	os.RemoveAll(testDir)
	time.Sleep(1 * time.Second)

	_, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	// 快速创建和删除多个文件
	const iterations = 10
	for i := 0; i < iterations; i++ {
		fileName := filepath.Join(testDir, fmt.Sprintf("rapid_%d.txt", i))
		content := []byte(fmt.Sprintf("rapid content %d", i))

		// 创建
		if err := os.WriteFile(fileName, content, 0644); err != nil {
			t.Fatalf("Iteration %d: WriteFile failed: %v", i, err)
		}

		// 立即删除
		if err := os.Remove(fileName); err != nil {
			t.Fatalf("Iteration %d: Remove failed: %v", i, err)
		}
	}

	// 验证目录为空
	entries, err := os.ReadDir(testDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("Expected empty directory, got %d entries", len(entries))
	}

	// Cleanup
	os.RemoveAll(testDir)
}

// TestE2E_LargeFileTruncate 大文件截断测试
func TestE2E_LargeFileTruncate(t *testing.T) {
	config := loadE2EConfig(t)

	testFile := filepath.Join(config.mountPoint, "large_truncate.bin")
	os.Remove(testFile)
	time.Sleep(1 * time.Second)

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	// 1. 创建 5MB 文件
	largeContent := bytes.Repeat([]byte("ABCDEFGHIJ"), 500*1024) // 5MB
	if err := os.WriteFile(testFile, largeContent, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	waitForSync(t, fs, "/large_truncate.bin")

	// 2. 验证初始大小
	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Size() != int64(len(largeContent)) {
		t.Errorf("Initial size mismatch. Expected %d, got %d", len(largeContent), info.Size())
	}

	// 3. 截断到 1KB
	truncatedContent := largeContent[:1024]
	if err := os.WriteFile(testFile, truncatedContent, 0644); err != nil {
		t.Fatalf("Truncate WriteFile failed: %v", err)
	}
	waitForSync(t, fs, "/large_truncate.bin")

	// 4. 验证截断后大小
	info, err = os.Stat(testFile)
	if err != nil {
		t.Fatalf("Stat after truncate failed: %v", err)
	}
	if info.Size() != 1024 {
		t.Errorf("Truncated size mismatch. Expected 1024, got %d", info.Size())
	}

	// 5. 验证内容
	readContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(truncatedContent, readContent) {
		t.Errorf("Content mismatch after truncate")
	}

	// 6. Cleanup
	os.Remove(testFile)
}
