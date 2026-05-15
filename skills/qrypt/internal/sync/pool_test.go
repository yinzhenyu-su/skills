package sync

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
)

type mockPoolDriver struct {
	drive.Driver
	mu      sync.Mutex
	entries map[string][]drive.Entry
	uploads map[string]bool
}

func (m *mockPoolDriver) List(ctx context.Context, parentID string) ([]drive.Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.entries[parentID], nil
}

func (m *mockPoolDriver) Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (drive.Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.uploads[name] = true
	return drive.Entry{ID: "new_id", Name: name, Size: size}, nil
}

func (m *mockPoolDriver) Mkdir(ctx context.Context, parentID, name string) (drive.Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := drive.Entry{ID: "dir_" + name, Name: name, IsDir: true}
	m.entries[parentID] = append(m.entries[parentID], entry)
	return entry, nil
}

func TestWorkerPool_UploadUpdateSkip(t *testing.T) {
	cipher, _ := crypt.NewRcloneCipher("password", "salt")
	
	// Create local test files
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")
	
	os.WriteFile(file1, []byte("same size"), 0644)
	os.WriteFile(file2, []byte("different size data"), 0644)
	
	f1Info, _ := os.Stat(file1)
	
	encName1 := cipher.EncryptSegment("file1.txt")
	encSize1 := cipher.EncryptedSize(f1Info.Size())

	// Mock driver where file1 already exists with SAME size, file2 doesn't exist
	mDrv := &mockPoolDriver{
		entries: map[string][]drive.Entry{
			"root": {
				{ID: "f1", Name: encName1, Size: encSize1, IsDir: false},
			},
		},
		uploads: make(map[string]bool),
	}

	// Update is TRUE
	pool := NewWorkerPool(mDrv, cipher, 2, false, true)
	pool.Start(context.Background())

	err := ScanLocalForUpload(context.Background(), tmpDir, "root", mDrv, cipher, pool)
	if err != nil {
		t.Fatalf("ScanLocalForUpload failed: %v", err)
	}

	pool.Wait()

	mDrv.mu.Lock()
	defer mDrv.mu.Unlock()

	// file1 should be skipped because --update is true and size matches
	if mDrv.uploads["file1.txt"] {
		t.Errorf("file1.txt should have been skipped, but was uploaded")
	}

	encName2 := cipher.EncryptSegment("file2.txt")
	if !mDrv.uploads[encName2] {
		t.Errorf("file2.txt should have been uploaded")
	}
}
