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
	// Use t.Name() which is unique enough
	config.remotePath = filepath.Join(config.remotePath, t.Name())
	
	// Use a short TTL
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

	const fileName = "conflict_test.txt"
	filePath := filepath.Join(config.mountPoint, fileName)
	
	// 1. Create file and sync it
	initialContent := []byte("initial content")
	if err := os.WriteFile(filePath, initialContent, 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}
	waitForSync(t, fs, "/"+fileName)
	
	// 2. Modify on remote directly
	remoteContent := []byte("modified on remote [longer]")
	uploadRemoteFile(t, config, fileName, remoteContent)
	
	// 3. Modify locally via FUSE
	localContent := []byte("modified locally [shorter]")
	f, err := os.OpenFile(filePath, os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("Failed to open file for local edit: %v", err)
	}
	_, err = f.Write(localContent)
	if err != nil {
		t.Fatalf("Failed to write local changes: %v", err)
	}
	_ = f.Truncate(int64(len(localContent)))
	
	// 4. Trigger sync (via Close/Flush)
	if err := f.Close(); err != nil {
		t.Fatalf("Failed to close file: %v", err)
	}
	
	// 5. Wait for background sync to detect conflict and resolve it
	time.Sleep(20 * time.Second)
	
	// 6. Verify result
	_, _ = os.ReadDir(config.mountPoint) // Trigger refresh
	
	readOriginal, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read original file after conflict: %v", err)
	}
	if !bytes.Equal(remoteContent, readOriginal) {
		t.Errorf("Original file should have remote content. Got %q, want %q", string(readOriginal), string(remoteContent))
	}
	
	// There should be a conflict file with local changes
	entries, err := os.ReadDir(config.mountPoint)
	if err != nil {
		t.Fatalf("Failed to read mount point: %v", err)
	}
	
	foundConflict := false
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "[Local Conflict") {
			foundConflict = true
			conflictPath := filepath.Join(config.mountPoint, entry.Name())
			readConflict, err := os.ReadFile(conflictPath)
			if err != nil {
				t.Fatalf("Failed to read conflict file: %v", err)
			}
			if !bytes.Equal(localContent, readConflict) {
				t.Errorf("Conflict file content mismatch. Got %q, want %q", string(readConflict), string(localContent))
			}
			break
		}
	}
	
	if !foundConflict {
		t.Error("Expected conflict file was not found")
	}
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
