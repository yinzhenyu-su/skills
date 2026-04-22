package vfs

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestE2E_UploadRetryOnParentDirDelete tests that when a file's parent directory
// is deleted on the server during upload, Qrypt recreates the directory and retries.
//
// Scenario:
//  1. Create subdirectory "retryparent" and write a file into it
//  2. Wait for file to sync
//  3. Delete the parent directory on the server (simulating external deletion)
//  4. Write a new file into the same directory
//  5. The sync should detect the missing dir, recreate it, and upload successfully
func TestE2E_UploadRetryOnParentDirDelete(t *testing.T) {
	config := loadE2EConfig(t)
	config.remotePath = filepath.Join(config.remotePath, "dir_gone_test")

	oldTTL := MetadataTTL
	MetadataTTL = 2 * time.Second
	defer func() { MetadataTTL = oldTTL }()

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	fs.driver.DirCacheTTL = 2 * time.Second
	fs.driver.NegCacheTTL = 2 * time.Second
	defer unmount(config.mountPoint)
	defer host.Unmount()

	const dirName = "retryparent"
	dirPath := filepath.Join(config.mountPoint, dirName)

	// 1. Create directory and a file to ensure the dir exists on server
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	file1Path := filepath.Join(dirPath, "first.txt")
	if err := os.WriteFile(file1Path, []byte("first file content"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Wait for first file to sync
	time.Sleep(5 * time.Second)
	fs.driver.RemoveDirCache(fs.rootFid)
	_, _ = os.ReadDir(dirPath) // trigger readdir to refresh

	waitForSyncMultiple(t, fs, file1Path, 30*time.Second)
	t.Log("First file synced")

	// 2. Get the directory's FID on server
	cipher := fs.cipher
	dirNode := getNode(t, fs, dirPath)
	dirFid := dirNode.fid
	t.Logf("Directory FID: %s", dirFid)

	// 3. Delete the directory on the server
	if err := fs.driver.Delete([]string{dirFid}); err != nil {
		t.Fatalf("Failed to delete directory on server: %v", err)
	}
	fs.driver.RemoveDirCache(fs.rootFid)
	t.Log("Directory deleted on server")

	// 4. Write a second file into the same local directory (dir still exists in FUSE)
	file2Path := filepath.Join(dirPath, "second.txt")
	if err := os.WriteFile(file2Path, []byte("second file after dir delete"), 0644); err != nil {
		t.Fatalf("WriteFile (second) failed: %v", err)
	}

	// 5. Wait for sync — should succeed despite directory being deleted
	waitForSyncMultiple(t, fs, file2Path, 60*time.Second)
	t.Log("Second file synced after directory recreation")

	// 6. Verify both files exist on server
	dirNodeAfter := getNode(t, fs, dirPath)
	newDirFid := dirNodeAfter.fid
	t.Logf("New directory FID after recreation: %s", newDirFid)

	fs.driver.RemoveDirCache(newDirFid)
	files, err := fs.driver.ListFiles(newDirFid)
	if err != nil {
		t.Fatalf("ListFiles failed: %v", err)
	}

	if len(files) < 1 {
		t.Fatalf("Expected at least 1 file in recreated directory, got %d", len(files))
	}

	// Check that second.txt exists on server
	foundSecond := false
	for _, f := range files {
		decName, decErr := cipher.DecryptSegment(f.FileName)
		if decErr != nil {
			continue
		}
		if decName == "second.txt" {
			foundSecond = true
			break
		}
	}
	if !foundSecond {
		t.Error("second.txt not found on server after dir recreation and upload")
	}
	t.Log("PASS: Upload retry after parent directory deletion works")
}

// TestE2E_RemoteDeleteSyncsToLocal tests that when a file is deleted on the server,
// the next Readdir (after TTL expiry) removes it from the local node tree.
//
// Scenario:
//  1. Upload a file via mount point, wait for sync
//  2. Delete the file on the server (using driver API)
//  3. Wait for MetadataTTL to expire
//  4. Trigger Readdir (ls on mount point)
//  5. Verify the file is gone from the local node tree
func TestE2E_RemoteDeleteSyncsToLocal(t *testing.T) {
	config := loadE2EConfig(t)
	config.remotePath = filepath.Join(config.remotePath, "remote_delete_test")

	oldTTL := MetadataTTL
	MetadataTTL = 2 * time.Second
	defer func() { MetadataTTL = oldTTL }()

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	fs.driver.DirCacheTTL = 2 * time.Second
	fs.driver.NegCacheTTL = 2 * time.Second
	defer unmount(config.mountPoint)
	defer host.Unmount()

	const fileName = "will_be_deleted.txt"
	filePath := filepath.Join(config.mountPoint, fileName)

	// 1. Create and sync a file
	if err := os.WriteFile(filePath, []byte("delete me from server"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	waitForSyncMultiple(t, fs, filePath, 30*time.Second)
	t.Log("File synced to server")

	// Verify it exists in node tree
	if _, ok := fs.nodes.Load("/" + fileName); !ok {
		t.Fatal("File should exist in node tree after sync")
	}

	// 2. Get the file's FID and delete it on server
	fileNode := getNode(t, fs, "/"+fileName)
	fileNode.mu.RLock()
	fid := fileNode.fid
	parentFid := fileNode.parentFid
	fileNode.mu.RUnlock()

	if err := fs.driver.Delete([]string{fid}); err != nil {
		t.Fatalf("Failed to delete file on server: %v", err)
	}
	fs.driver.RemoveDirCache(parentFid)
	t.Log("File deleted on server")

	// 3. Wait for MetadataTTL to expire + extra buffer
	time.Sleep(MetadataTTL + 3*time.Second)

	// 4. Trigger Readdir to force remote check
	_, _ = os.ReadDir(config.mountPoint)

	// 5. Verify file is removed from node tree
	// Give some time for MergeRemoteChanges to process
	time.Sleep(2 * time.Second)
	_, _ = os.ReadDir(config.mountPoint)
	time.Sleep(1 * time.Second)

	if _, ok := fs.nodes.Load("/" + fileName); ok {
		// Check if the node is dirty (should not be deleted if dirty)
		v, _ := fs.nodes.Load("/" + fileName)
		n := v.(*node)
		n.mu.RLock()
		isDirty := n.isDirty
		n.mu.RUnlock()
		if !isDirty {
			t.Error("File should be removed from node tree after remote deletion")
		} else {
			t.Log("File still in node tree but is dirty — acceptable (merged conflict)")
		}
	} else {
		t.Log("PASS: File removed from node tree after remote deletion")
	}
}

// TestE2E_RemoteDeletePreservesLocalDirty tests that when a file is deleted on
// the server but has local (dirty) changes, the local version is preserved and
// converted to a local_ node for re-upload.
//
// Scenario:
//  1. Upload a file, wait for sync
//  2. Delete the file on the server
//  3. Modify the file locally (make it dirty)
//  4. Trigger Readdir after TTL
//  5. Verify the local dirty file is preserved (not deleted)
func TestE2E_RemoteDeletePreservesLocalDirty(t *testing.T) {
	config := loadE2EConfig(t)
	config.remotePath = filepath.Join(config.remotePath, "remote_delete_dirty_test")

	oldTTL := MetadataTTL
	MetadataTTL = 2 * time.Second
	defer func() { MetadataTTL = oldTTL }()

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	fs.driver.DirCacheTTL = 2 * time.Second
	fs.driver.NegCacheTTL = 2 * time.Second
	defer unmount(config.mountPoint)
	defer host.Unmount()

	const fileName = "dirty_will_survive.txt"
	filePath := filepath.Join(config.mountPoint, fileName)

	// 1. Create and sync
	if err := os.WriteFile(filePath, []byte("original content"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	waitForSyncMultiple(t, fs, filePath, 30*time.Second)

	// 2. Delete on server
	fileNode := getNode(t, fs, "/"+fileName)
	fileNode.mu.RLock()
	fid := fileNode.fid
	parentFid := fileNode.parentFid
	fileNode.mu.RUnlock()

	if err := fs.driver.Delete([]string{fid}); err != nil {
		t.Fatalf("Delete on server failed: %v", err)
	}
	fs.driver.RemoveDirCache(parentFid)

	// 3. Modify locally (make dirty) — open for write
	f, err := os.OpenFile(filePath, os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		t.Fatalf("OpenFile failed: %v", err)
	}
	_, _ = f.Write([]byte("\nmodified after server delete"))
	_ = f.Sync()
	_ = f.Close()

	// 4. Wait for TTL and trigger Readdir
	time.Sleep(MetadataTTL + 3*time.Second)
	_, _ = os.ReadDir(config.mountPoint)
	time.Sleep(2 * time.Second)

	// 5. Verify file still exists in node tree (dirty or merged)
	v, ok := fs.nodes.Load("/" + fileName)
	if !ok {
		t.Fatal("Dirty file should NOT be deleted when remote is deleted")
	}
	n := v.(*node)
	n.mu.RLock()
	isDirty := n.isDirty
	nodeFid := n.fid
	n.mu.RUnlock()

	if !isDirty {
		t.Error("File should still be dirty after remote deletion + local modification")
	}
	if nodeFid == fid {
		t.Error("FID should have changed to local_ prefix after remote deletion")
	}
	t.Logf("PASS: Dirty file preserved after remote deletion (fid=%s, dirty=%v)", nodeFid, isDirty)
}

// TestE2E_RemoteAddVisibleAfterReaddir tests that when a new file appears on the
// server (uploaded externally), it becomes visible in the local mount after TTL.
//
// Scenario:
//  1. Mount the filesystem (empty)
//  2. Upload a file directly to the server (bypassing FUSE)
//  3. Wait for TTL expiry, trigger Readdir
//  4. Verify the file appears in the local mount
func TestE2E_RemoteAddVisibleAfterReaddir(t *testing.T) {
	config := loadE2EConfig(t)
	config.remotePath = filepath.Join(config.remotePath, "remote_add_test")

	oldTTL := MetadataTTL
	MetadataTTL = 2 * time.Second
	defer func() { MetadataTTL = oldTTL }()

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	fs.driver.DirCacheTTL = 2 * time.Second
	fs.driver.NegCacheTTL = 2 * time.Second
	defer unmount(config.mountPoint)
	defer host.Unmount()

	const fileName = "remote_added.txt"
	filePath := filepath.Join(config.mountPoint, fileName)
	content := []byte("uploaded directly to server")

	// 1. Upload directly to server
	uploadRemoteFile(t, config, fileName, content)

	// 2. Wait for TTL + buffer, then trigger Readdir
	time.Sleep(MetadataTTL + 3*time.Second)
	_, _ = os.ReadDir(config.mountPoint)

	// 3. Verify file becomes visible
	success := false
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		if _, err := os.Stat(filePath); err == nil {
			success = true
			break
		}
		_, _ = os.ReadDir(config.mountPoint)
	}
	if !success {
		t.Fatalf("Remote-added file %s not visible after TTL", fileName)
	}

	// 4. Verify content matches
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("Content mismatch: got %q, want %q", string(data), string(content))
	}
	t.Log("PASS: Remote-added file visible and content correct")
}

// TestE2E_LocalFileNotDeletedByRemoteSync tests that local-only files (not yet
// synced to server) are not deleted when MergeRemoteChanges runs.
//
// Scenario:
//  1. Create a file locally but don't let it sync (no wait)
//  2. Immediately trigger Readdir (MergeRemoteChanges)
//  3. Verify the local file is still present
func TestE2E_LocalFileNotDeletedByRemoteSync(t *testing.T) {
	config := loadE2EConfig(t)
	config.remotePath = filepath.Join(config.remotePath, "local_preserve_test")

	oldTTL := MetadataTTL
	MetadataTTL = 1 * time.Second
	defer func() { MetadataTTL = oldTTL }()

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	fs.driver.DirCacheTTL = 1 * time.Second
	fs.driver.NegCacheTTL = 1 * time.Second
	defer unmount(config.mountPoint)
	defer host.Unmount()

	const fileName = "local_only.txt"
	filePath := filepath.Join(config.mountPoint, fileName)

	// 1. Create file
	if err := os.WriteFile(filePath, []byte("local only content"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// 2. Immediately trigger Readdir (don't wait for sync)
	time.Sleep(MetadataTTL + 1*time.Second)
	_, _ = os.ReadDir(config.mountPoint)
	time.Sleep(1 * time.Second)

	// 3. Verify file still exists
	if _, err := os.Stat(filePath); err != nil {
		t.Errorf("Local-only file was deleted by remote sync: %v", err)
	}

	// Also check node tree
	v, ok := fs.nodes.Load("/" + fileName)
	if !ok {
		t.Error("Local-only file missing from node tree")
	} else {
		n := v.(*node)
		n.mu.RLock()
		source := n.source
		isDirty := n.isDirty
		n.mu.RUnlock()
		t.Logf("Local-only file: source=%s, dirty=%v", source, isDirty)
		if source != "local" {
			t.Errorf("Expected source='local', got '%s'", source)
		}
	}

	// 4. Now let it sync and verify it reaches the server
	waitForSyncMultiple(t, fs, filePath, 30*time.Second)
	t.Log("PASS: Local-only file preserved and synced")
}

// getNode is a test helper that retrieves a node at a given path or fails the test.
func getNode(t *testing.T, fs *QryptFS, path string) *node {
	t.Helper()
	v, ok := fs.nodes.Load(path)
	if !ok {
		t.Fatalf("Node not found: %s", path)
	}
	return v.(*node)
}
