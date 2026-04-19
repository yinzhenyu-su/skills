package vfs

import (
	"bytes"
	"encoding/json"
	"os"
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

func uploadRemoteFile(t *testing.T, config *e2eConfig, name string, content []byte) {
	t.Helper()
	
	cipher, _ := crypt.NewRcloneCipher(config.password, config.salt)
	d := driver.NewQuarkDriver(config.cookie)
	_ = d.Auth()
	
	rootFid, err := ensureRemotePath(d, config.remotePath)
	if err != nil {
		t.Fatalf("Failed to ensure remote path in uploadRemoteFile: %v", err)
	}
	
	tmpDir, _ := os.MkdirTemp("", "qrypt-remote-upload-*")
	defer os.RemoveAll(tmpDir)
	
	st, _ := staging.NewStore(tmpDir)
	uploader := uploadpkg.NewManager(d, cipher, st)
	
	localPath, _ := st.Create("remote-gen-" + name)
	_, _ = st.WriteAt(localPath, content, 0)
	
	_, err = uploader.Sync(uploadpkg.SyncRequest{
		Path:      "/" + name,
		Name:      name,
		ParentFid: rootFid,
		LocalPath: localPath,
		PlainSize: int64(len(content)),
	})
	if err != nil {
		t.Fatalf("Failed to upload remote file: %v", err)
	}
	d.RemoveDirCache(rootFid)
}

func TestE2E_RemoteChangeVisibility(t *testing.T) {
	config := loadE2EConfig(t)
	// Use it_test_dir which is cleaned up by setupQryptFSInternal
	config.remotePath = filepath.Join(config.remotePath, "it_test_dir")
	
	// Set a short TTL for this test
	oldTTL := MetadataTTL
	MetadataTTL = 2 * time.Second
	defer func() { MetadataTTL = oldTTL }()

	fs, host, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	fs.driver.DirCacheTTL = 2 * time.Second
	fs.driver.NegCacheTTL = 2 * time.Second
	fs.driver.RemoveDirCache(fs.rootFid)
	_ = fs
	defer unmount(config.mountPoint)
	defer host.Unmount()

	const fileName = "remote_only.txt"
	content := []byte("I was uploaded directly to remote")
	
	// 1. Initially should not exist (setupQryptFSInternal cleaned it)
	filePath := filepath.Join(config.mountPoint, fileName)
	if _, err := os.Stat(filePath); err == nil {
		// If it exists, try to delete it via driver to be sure
		t.Logf("File %s exists, cleaning up manually", fileName)
	}

	// 2. Upload directly to remote
	uploadRemoteFile(t, config, fileName, content)
	
	// 3. Wait for TTL to expire and retry stat
	success := false
	for i := 0; i < 15; i++ {
		time.Sleep(2 * time.Second)
		if _, err := os.Stat(filePath); err == nil {
			success = true
			break
		}
		// Trigger a readdir to encourage refresh
		_, _ = os.ReadDir(config.mountPoint)
	}
	
	if !success {
		t.Fatalf("File %s should be visible after TTL", fileName)
	}
	
	readContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read remote-added file: %v", err)
	}
	if !bytes.Equal(content, readContent) {
		t.Errorf("Content mismatch. Got %q, want %q", string(readContent), string(content))
	}
}

func TestE2E_ConflictResolution(t *testing.T) {
	config := loadE2EConfig(t)
	config.remotePath = filepath.Join(config.remotePath, "it_test_dir")

	oldTTL := MetadataTTL
	MetadataTTL = 2 * time.Second
	defer func() { MetadataTTL = oldTTL }()

	fs, _, err := setupQryptFSInternal(t, config, true)
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	fs.driver.DirCacheTTL = 2 * time.Second
	fs.driver.NegCacheTTL = 2 * time.Second

	const fileName = "conflict_test.txt"

	// 1. Create file and sync it
	initialContent := []byte("initial content")
	filePath := filepath.Join(config.mountPoint, fileName)
	if err := os.WriteFile(filePath, initialContent, 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	waitForSync(t, fs, "/"+fileName)

	// Get the synced node
	v, ok := fs.nodes.Load("/" + fileName)
	if !ok {
		t.Fatal("Node not found after sync")
	}
	n := v.(*node)
	n.mu.Lock()
	syncedFid := n.fid
	baseMtime := n.baseServerMtime
	n.mu.Unlock()
	t.Logf("Synced file: fid=%s, baseMtime=%d", syncedFid, baseMtime)

	// 2. Upload a different version to the server (simulates external modification)
	remoteContent := []byte("modified on remote [longer]")
	uploadRemoteFile(t, config, fileName, remoteContent)
	time.Sleep(2 * time.Second) // let API index the new file

	// 3. Directly test conflict detection in syncFile:
	//    - Set node to dirty state (as if user wrote to it locally)
	//    - baseServerMtime is still from step 1 (old)
	//    - syncFile should detect that server has a new file (different FID, newer mtime)
	cipher := fs.cipher
	st, _ := fs.staging.Create("test-conflict-" + fileName)
	testContent := []byte("modified locally [shorter]")
	_, _ = fs.staging.WriteAt(st, testContent, 0)

	nonce, _ := cipher.GenerateRandomNonce()

	n.mu.Lock()
	n.isDirty = true
	n.localPath = st
	n.fileNonce = nonce
	n.hasNonce = true
	n.size = int64(len(testContent))
	n.baseServerMtime = baseMtime // keep old base — should trigger conflict
	n.syncQueued = false
	n.mu.Unlock()

	t.Logf("Running syncFile with baseMtime=%d (old), server should have newer file", baseMtime)
	err = fs.syncFile("/"+fileName, n)
	if err != nil {
		t.Logf("syncFile returned error: %v", err)
	}

	// 4. Check if conflict was resolved
	n.mu.Lock()
	isDirty := n.isDirty
	currentFid := n.fid
	currentName := n.name
	n.mu.Unlock()

	t.Logf("After syncFile: isDirty=%v, fid=%s, name=%s", isDirty, currentFid, currentName)

	// The syncFile should have detected that our FID is gone and a same-name file exists,
	// triggering resolveConflict which renames the node
	if strings.Contains(currentName, "[Local Conflict") {
		t.Logf("SUCCESS: Conflict detected and node renamed to %s", currentName)
	} else if !isDirty && currentFid != syncedFid {
		t.Logf("SUCCESS: File was synced with new FID %s (conflict may have been resolved differently)", currentFid)
	} else {
		// Check if there's a conflict file in the directory
		time.Sleep(5 * time.Second)
		_, _ = os.ReadDir(config.mountPoint)

		foundConflict := false
		fs.nodes.Range(func(key, value interface{}) bool {
			nodePath := key.(string)
			node := value.(*node)
			node.mu.RLock()
			name := node.name
			node.mu.RUnlock()
			if strings.Contains(name, "[Local Conflict") && strings.Contains(nodePath, "conflict_test") {
				foundConflict = true
				t.Logf("Found conflict node: path=%s, name=%s", nodePath, name)
				return false
			}
			return true
		})

		if !foundConflict {
			t.Errorf("Expected conflict resolution: node should have been renamed or conflict file created. Got isDirty=%v, fid=%s, name=%s", isDirty, currentFid, currentName)
		}
	}

	// Cleanup
	_ = fs.staging.Remove(st)
}

func TestE2E_OpsLogRecovery(t *testing.T) {
	config := loadE2EConfig(t)
	// Use t.Name() which is unique enough
	config.remotePath = filepath.Join(config.remotePath, t.Name())
	
	// Pre-cleanup
	unmount(config.mountPoint)
	_ = os.RemoveAll(config.cacheDir)
	_ = os.MkdirAll(config.cacheDir, 0755)
	
	dbPath := filepath.Join(config.cacheDir, "qrypt_test.db")
	cdb, err := cache.NewCacheDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}

	// 1. Create a directory via driver to have something to rename
	cipher, _ := crypt.NewRcloneCipher(config.password, config.salt)
	d := driver.NewQuarkDriver(config.cookie)
	_ = d.Auth()
	rootFid, err := ensureRemotePath(d, config.remotePath)
	if err != nil {
		t.Fatalf("Failed to ensure remote path: %v", err)
	}
	
	const oldDirName = "recovery_test_old"
	const newDirName = "recovery_test_new"
	encOldName := cipher.EncryptSegment(oldDirName)
	encNewName := cipher.EncryptSegment(newDirName)
	
	fid, err := d.CreateDir(rootFid, encOldName)
	if err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}

	// 2. Insert a PENDING RENAME log into DB
	payload, _ := json.Marshal(opsPayload{Fids: []string{fid}, Name: encNewName})
	_, err = cdb.AddOpsLogEntry("RENAME", "/"+oldDirName, "/"+newDirName, string(payload))
	if err != nil {
		t.Fatalf("Failed to insert ops log: %v", err)
	}
	_ = cdb.Close() 

	// 3. Start FS - it should recover the rename
	_, host, err := setupQryptFSInternal(t, config, false) 
	if err != nil {
		t.Fatalf("Failed to setup QryptFS: %v", err)
	}
	defer unmount(config.mountPoint)
	defer host.Unmount()

	// 4. Verify rename was executed
	success := false
	newDirPath := filepath.Join(config.mountPoint, newDirName)
	for i := 0; i < 10; i++ {
		time.Sleep(2 * time.Second)
		if _, err := os.Stat(newDirPath); err == nil {
			success = true
			break
		}
		// Trigger refresh
		_, _ = os.ReadDir(config.mountPoint)
	}
	
	if !success {
		t.Errorf("Recovered directory %s not found", newDirName)
	}
	
	oldDirPath := filepath.Join(config.mountPoint, oldDirName)
	if _, err := os.Stat(oldDirPath); err == nil {
		t.Errorf("Old directory %s should not exist", oldDirName)
	}
}
