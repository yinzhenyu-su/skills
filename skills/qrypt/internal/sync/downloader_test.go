package sync

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
)

// mockDriver is a simple mock for testing the Downloader.
type mockDownloaderDriver struct {
	drive.Driver // embedded to satisfy interface, we only override Read
	fileData     map[string][]byte
}

func (m *mockDownloaderDriver) Read(ctx context.Context, entry drive.Entry, offset, size int64) (io.ReadCloser, error) {
	data, ok := m.fileData[entry.ID]
	if !ok {
		return nil, os.ErrNotExist
	}
	if offset > int64(len(data)) {
		return nil, io.EOF
	}
	end := offset + size
	if end > int64(len(data)) {
		end = int64(len(data))
	}
	return io.NopCloser(bytes.NewReader(data[offset:end])), nil
}

func TestDownloader_Download(t *testing.T) {
	cipher, err := crypt.NewRcloneCipher("password", "salt")
	if err != nil {
		t.Fatalf("Failed to create cipher: %v", err)
	}

	testData := []byte("Hello, this is a test payload for the downloader!")
	
	// Create mock encrypted file in memory
	nonce, _ := cipher.GenerateRandomNonce()
	encBuf := new(bytes.Buffer)
	
	// Write magic
	encBuf.Write([]byte(crypt.FileMagic))
	encBuf.Write(nonce[:])
	
	// Encrypt body
	blockIndex := uint64(0)
	encBlock, _ := cipher.EncryptBlock(testData, blockIndex, nonce)
	encBuf.Write(encBlock)

	encData := encBuf.Bytes()

	mDrv := &mockDownloaderDriver{
		fileData: map[string][]byte{
			"file1": encData,
		},
	}

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "out.txt")

	downloader := NewDownloader(mDrv, cipher)

	req := DownloadRequest{
		Entry: drive.Entry{
			ID:   "file1",
			Size: int64(len(encData)),
		},
		LocalPath: outPath,
	}

	err = downloader.Download(context.Background(), req)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	outBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	if string(outBytes) != string(testData) {
		t.Errorf("Decrypted data mismatch.\nGot: %s\nWant: %s", string(outBytes), string(testData))
	}
}
