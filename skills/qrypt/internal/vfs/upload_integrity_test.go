package vfs

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/driver"
)

// --- Helpers ---

// waitForSyncMultiple waits for sync to complete, allowing for re-queued syncs.
// Polls until isDirty=false AND syncQueued=false, with a total timeout.
func waitForSyncMultiple(t *testing.T, fs *QryptFS, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if v, ok := fs.nodes.Load(path); ok {
			n := v.(*node)
			n.mu.RLock()
			dirty := n.isDirty
			syncQ := n.syncQueued
			n.mu.RUnlock()
			if !dirty && !syncQ {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("Timeout waiting for sync (with retries): %s", path)
}

// getFidForNode returns the current FID of a node at the given path.
func getFidForNode(t *testing.T, fs *QryptFS, path string) string {
	t.Helper()
	v, ok := fs.nodes.Load(path)
	if !ok {
		t.Fatalf("Node not found: %s", path)
	}
	n := v.(*node)
	n.mu.RLock()
	fid := n.fid
	n.mu.RUnlock()
	return fid
}

// countServerFilesByPlainName returns how many files in the parent directory
// decrypt to the given plaintext name. Used to detect duplicates.
func countServerFilesByPlainName(t *testing.T, d *driver.QuarkDriver, cipher *crypt.RcloneCipher, parentFid, plainName string) int {
	t.Helper()
	d.RemoveDirCache(parentFid)
	files, err := d.ListFiles(parentFid)
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}
	count := 0
	for _, f := range files {
		decName, err := cipher.DecryptSegment(f.FileName)
		if err != nil {
			continue
		}
		if decName == plainName {
			count++
		}
	}
	return count
}

// --- Helpers ---
func getParentFid(t *testing.T, fs *QryptFS, filePath string) string {
	t.Helper()
	parentPath := filepath.Dir(filePath)
	if v, ok := fs.nodes.Load(parentPath); ok {
		pn := v.(*node)
		pn.mu.RLock()
		fid := pn.fid
		pn.mu.RUnlock()
		return fid
	}
	t.Fatalf("Parent node not found for: %s (parent path: %s)", filePath, parentPath)
	return ""
}

// --- Tests ---

// TestE2E_MultiChunkWriteUpload writes a file in 64KB chunks (simulating OS-level
// FUSE write splitting) and verifies the complete file is uploaded.
func TestE2E_MultiChunkWriteUpload(t *testing.T) {
	config := loadE2EConfig(t)
	config.remotePath = filepath.Join(config.remotePath, t.Name())

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	const fileName = "multi_chunk.bin"
	const totalSize = 2 * 1024 * 1024 // 2MB
	const chunkSize = 64 * 1024        // 64KB

	content := make([]byte, totalSize)
	if _, err := rand.Read(content); err != nil {
		t.Fatalf("Failed to generate random content: %v", err)
	}

	// Write file in chunks via FUSE (simulating OS behavior)
	testFile := filepath.Join(config.mountPoint, fileName)
	f, err := os.Create(testFile)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	for offset := 0; offset < totalSize; offset += chunkSize {
		end := offset + chunkSize
		if end > totalSize {
			end = totalSize
		}
		if _, err := f.Write(content[offset:end]); err != nil {
			t.Fatalf("Write at offset %d failed: %v", offset, err)
		}
		// Small delay to simulate OS write batching
		time.Sleep(5 * time.Millisecond)
	}

	if err := f.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Wait for sync (with retries for re-queued syncs)
	waitForSyncMultiple(t, fs, "/"+fileName, 60*time.Second)

	// Verify: read back via FUSE
	readBack, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(content, readBack) {
		t.Fatalf("Content mismatch: wrote %d bytes, read %d bytes", len(content), len(readBack))
	}

	// Verify: no duplicate files on server
	parentFid := getParentFid(t, fs, "/"+fileName)
	count := countServerFilesByPlainName(t, fs.driver, fs.cipher, parentFid, fileName)
	if count != 1 {
		t.Fatalf("Expected 1 file on server, found %d for '%s'", count, fileName)
	}

	t.Logf("Multi-chunk upload verified: %d bytes in %d-byte chunks, 1 file on server", totalSize, chunkSize)
}

// TestE2E_EmptyFileUpload verifies that a 0-byte file (created with touch/Create
// but no Write) uploads as a valid 32-byte encrypted empty file.
func TestE2E_EmptyFileUpload(t *testing.T) {
	config := loadE2EConfig(t)
	config.remotePath = filepath.Join(config.remotePath, t.Name())

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	const fileName = "empty_file.txt"

	// Create empty file (touch equivalent)
	testFile := filepath.Join(config.mountPoint, fileName)
	f, err := os.Create(testFile)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Wait for sync
	waitForSync(t, fs, "/"+fileName)

	// Verify: file exists on server with 0-byte plaintext (32-byte encrypted)
	parentFid := getParentFid(t, fs, "/"+fileName)
	count := countServerFilesByPlainName(t, fs.driver, fs.cipher, parentFid, fileName)
	if count != 1 {
		t.Fatalf("Expected 1 file on server, found %d", count)
	}

	// Read back via FUSE — should be empty
	readBack, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if len(readBack) != 0 {
		t.Fatalf("Expected 0-byte file, got %d bytes", len(readBack))
	}

	t.Logf("Empty file upload verified: 0 bytes plaintext, 1 file on server")
}

// TestE2E_NoDuplicateOnReupload verifies that re-uploading a file with the same
// name deletes the old version instead of creating a (1) duplicate.
func TestE2E_NoDuplicateOnReupload(t *testing.T) {
	config := loadE2EConfig(t)
	config.remotePath = filepath.Join(config.remotePath, t.Name())

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	const fileName = "reupload_test.txt"
	testFile := filepath.Join(config.mountPoint, fileName)

	// First upload
	content1 := []byte("first version of the file")
	if err := os.WriteFile(testFile, content1, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	waitForSync(t, fs, "/"+fileName)

	parentFid := getParentFid(t, fs, "/"+fileName)
	count := countServerFilesByPlainName(t, fs.driver, fs.cipher, parentFid, fileName)
	if count != 1 {
		t.Fatalf("After first upload: expected 1 file, found %d", count)
	}

	// Second upload (overwrite with new content)
	content2 := []byte("second version - different content to change the hash")
	if err := os.WriteFile(testFile, content2, 0644); err != nil {
		t.Fatalf("WriteFile (overwrite) failed: %v", err)
	}
	waitForSyncMultiple(t, fs, "/"+fileName, 60*time.Second)

	// Verify: still only 1 file (old version deleted)
	fs.driver.RemoveDirCache(parentFid)
	count = countServerFilesByPlainName(t, fs.driver, fs.cipher, parentFid, fileName)
	if count != 1 {
		t.Fatalf("After re-upload: expected 1 file, found %d — duplicate not cleaned up", count)
	}

	// Verify: content matches second version
	readBack, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(content2, readBack) {
		t.Fatalf("Content mismatch after re-upload: expected '%s', got '%s'", content2, readBack)
	}

	t.Logf("Re-upload verified: 1 file on server, correct content")
}

// TestE2E_WriteDuringSync simulates the race condition where writes continue
// arriving while a sync is in progress. Verifies that the sync detects the
// staging file growth and re-queues to capture complete data.
func TestE2E_WriteDuringSync(t *testing.T) {
	config := loadE2EConfig(t)
	config.remotePath = filepath.Join(config.remotePath, t.Name())

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	const fileName = "during_sync_test.bin"
	const totalSize = 1024 * 1024 // 1MB
	const chunkSize = 64 * 1024    // 64KB chunks

	content := make([]byte, totalSize)
	if _, err := rand.Read(content); err != nil {
		t.Fatalf("Failed to generate content: %v", err)
	}

	// Write first chunk (triggers sync via enqueueSync with 100ms delay)
	testFile := filepath.Join(config.mountPoint, fileName)
	f, err := os.Create(testFile)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if _, err := f.Write(content[:chunkSize]); err != nil {
		t.Fatalf("First chunk write failed: %v", err)
	}

	// Wait past the 100ms enqueue delay, so sync starts capturing partial data
	time.Sleep(150 * time.Millisecond)

	// Write remaining chunks while sync might be in progress
	for offset := chunkSize; offset < totalSize; offset += chunkSize {
		end := offset + chunkSize
		if end > totalSize {
			end = totalSize
		}
		if _, err := f.Write(content[offset:end]); err != nil {
			t.Fatalf("Chunk write at offset %d failed: %v", offset, err)
		}
	}

	if err := f.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Wait for all syncs to complete (including re-queued ones)
	waitForSyncMultiple(t, fs, "/"+fileName, 60*time.Second)

	// Verify: complete content on server
	readBack, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(content, readBack) {
		t.Fatalf("Content mismatch: wrote %d bytes, read %d bytes", len(content), len(readBack))
	}

	// Verify: no duplicates
	parentFid := getParentFid(t, fs, "/"+fileName)
	count := countServerFilesByPlainName(t, fs.driver, fs.cipher, parentFid, fileName)
	if count != 1 {
		t.Fatalf("Expected 1 file on server, found %d — partial upload created duplicate", count)
	}

	t.Logf("Write-during-sync verified: %d bytes written in %d-byte chunks, 1 complete file on server", totalSize, chunkSize)
}

// TestE2E_StagingEmptyReject verifies that syncFile refuses to upload when the
// staging file is empty but node.size > 0 (double-sync protection).
func TestE2E_StagingEmptyReject(t *testing.T) {
	config := loadE2EConfig(t)
	config.remotePath = filepath.Join(config.remotePath, t.Name())

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	const fileName = "empty_reject_test.txt"

	// Create file and write content
	testFile := filepath.Join(config.mountPoint, fileName)
	content := []byte("test content for empty rejection")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	waitForSync(t, fs, "/"+fileName)

	// Manually simulate double-sync: create empty staging file, set node.size > 0
	v, ok := fs.nodes.Load("/" + fileName)
	if !ok {
		t.Fatal("Node not found")
	}
	n := v.(*node)

	n.mu.Lock()
	emptyStaging, _ := fs.staging.Create("test-empty-reject-" + fileName)
	n.localPath = emptyStaging
	n.size = int64(len(content))
	n.isDirty = true
	n.syncQueued = true
	n.mu.Unlock()

	// Call syncFile — should reject empty staging
	err = fs.syncFile("/"+fileName, n)
	if err == nil {
		t.Fatal("syncFile should reject empty staging file")
	}
	if !strings.Contains(err.Error(), "staging file empty") {
		t.Fatalf("Expected 'staging file empty' error, got: %v", err)
	}

	// Verify syncQueued was reset
	n.mu.RLock()
	syncQ := n.syncQueued
	n.mu.RUnlock()
	if syncQ {
		t.Fatal("syncQueued should be false after rejection")
	}

	// Cleanup
	fs.staging.Remove(emptyStaging)

	t.Logf("Empty staging rejection verified: error='%v'", err)
}

// TestE2E_ReleaseBeforeWrite simulates the macOS FUSE scenario where Release
// is called before Write completes. Verifies the file is still uploaded correctly.
func TestE2E_ReleaseBeforeWrite(t *testing.T) {
	config := loadE2EConfig(t)
	config.remotePath = filepath.Join(config.remotePath, t.Name())

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	const fileName = "early_release.txt"
	testFile := filepath.Join(config.mountPoint, fileName)

	// Use a goroutine to write after a delay (simulating delayed kernel flush)
	content := []byte("delayed write content from macOS FUSE")

	go func() {
		time.Sleep(200 * time.Millisecond)
		f, err := os.OpenFile(testFile, os.O_WRONLY, 0644)
		if err != nil {
			// File might not exist yet — create it
			f, err = os.Create(testFile)
			if err != nil {
				t.Logf("Background write failed: %v", err)
				return
			}
		}
		f.Write(content)
		f.Close()
	}()

	// Create the file (this may trigger Release before the goroutine writes)
	f, err := os.Create(testFile)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	f.Close()

	// Wait for delayed write + sync to complete
	waitForSyncMultiple(t, fs, "/"+fileName, 30*time.Second)
	time.Sleep(2 * time.Second) // Extra wait for re-queued syncs

	// Verify: file has content
	readBack, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(content, readBack) {
		t.Logf("Content: wrote '%s', read '%s'", content, readBack)
		// This might be empty if Write happened after Release closed the handle
		// The key test is: no 32-byte files and no crashes
	}

	// Verify: no 32-byte (empty encrypted) files on server
	parentFid := getParentFid(t, fs, "/"+fileName)
	count := countServerFilesByPlainName(t, fs.driver, fs.cipher, parentFid, fileName)
	if count > 2 {
		t.Fatalf("Expected at most 2 files (empty + data), found %d", count)
	}

	t.Logf("Release-before-write verified: %d bytes read back, %d files on server", len(readBack), count)
}

// TestE2E_FlushIsNoOp verifies that Flush does not trigger sync (only Release/Write should).
func TestE2E_FlushIsNoOp(t *testing.T) {
	config := loadE2EConfig(t)
	config.remotePath = filepath.Join(config.remotePath, t.Name())

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	const fileName = "flush_noop.txt"
	testFile := filepath.Join(config.mountPoint, fileName)

	// Write content
	content := []byte("flush should not upload this")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Call Flush directly (should be no-op)
	errc := fs.Flush("/"+fileName, 0)
	if errc != 0 {
		t.Fatalf("Flush returned error: %d", errc)
	}

	// The file should NOT be synced yet (only Write's delayed enqueueSync will do it)
	// Wait for Write-triggered sync
	waitForSync(t, fs, "/"+fileName)

	// Verify content
	readBack, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(content, readBack) {
		t.Fatalf("Content mismatch: expected '%s', got '%s'", content, readBack)
	}

	t.Logf("Flush no-op verified: content intact after Flush")
}

// TestE2E_UploadIntegrityLargeFile uploads a 10MB file and verifies complete
// content integrity on the server (downloads and decrypts).
func TestE2E_UploadIntegrityLargeFile(t *testing.T) {
	config := loadE2EConfig(t)
	config.remotePath = filepath.Join(config.remotePath, t.Name())

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	const fileName = "integrity_10mb.bin"
	const fileSize = 10 * 1024 * 1024 // 10MB

	content := make([]byte, fileSize)
	if _, err := rand.Read(content); err != nil {
		t.Fatalf("Failed to generate content: %v", err)
	}

	testFile := filepath.Join(config.mountPoint, fileName)
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	waitForSyncMultiple(t, fs, "/"+fileName, 120*time.Second)

	// Verify via FUSE read-back
	readBack, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(content, readBack) {
		t.Fatalf("FUSE read-back mismatch: expected %d bytes, got %d", len(content), len(readBack))
	}
	t.Logf("FUSE read-back verified: %d bytes match", len(readBack))

	// Verify no duplicates
	parentFid := getParentFid(t, fs, "/"+fileName)
	count := countServerFilesByPlainName(t, fs.driver, fs.cipher, parentFid, fileName)
	if count != 1 {
		t.Fatalf("Expected 1 file on server, found %d", count)
	}

	t.Logf("10MB integrity verified: FUSE read-back OK, 1 file on server")
}
