package upload

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/yinzhenyu/skills/qrypt/cipher"
	"github.com/yinzhenyu/skills/qrypt/drivers"
)

type mockUploaderDriver struct {
	drivers.Driver
	putFn func(ctx context.Context, parentID, name string, size int64, body io.Reader) (drivers.Entry, error)
}

func (m *mockUploaderDriver) Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (drivers.Entry, error) {
	return m.putFn(ctx, parentID, name, size, body)
}

type ucipherAdapter struct {
	inner *cipher.RcloneCipher
}

func (a *ucipherAdapter) EncryptSegment(plain string) string { return a.inner.EncryptSegment(plain) }
func (a *ucipherAdapter) DecryptSegment(ct string) (string, error) { return a.inner.DecryptSegment(ct) }
func (a *ucipherAdapter) EncryptBlock(plaintext []byte, blockIndex uint64, nonce [cipher.FileNonceSize]byte) ([]byte, error) {
	return a.inner.EncryptBlock(plaintext, blockIndex, nonce)
}
func (a *ucipherAdapter) DecryptBlock(ciphertext []byte, blockIndex uint64, nonce [cipher.FileNonceSize]byte) ([]byte, error) {
	return a.inner.DecryptBlock(ciphertext, blockIndex, nonce)
}
func (a *ucipherAdapter) EncryptedSize(plainSize int64) int64 { return a.inner.EncryptedSize(plainSize) }
func (a *ucipherAdapter) DecryptedSize(cipherSize int64) (int64, error) { return a.inner.DecryptedSize(cipherSize) }
func (a *ucipherAdapter) GenerateRandomNonce() ([cipher.FileNonceSize]byte, error) { return a.inner.GenerateRandomNonce() }

func newTestCipher() cipher.Cipher {
	c, _ := cipher.NewRcloneCipher("password", "")
	return &ucipherAdapter{inner: c}
}

func TestNewUploader(t *testing.T) {
	drv := &mockUploaderDriver{}
	u := NewUploader(drv, newTestCipher())
	if u == nil {
		t.Fatal("expected non-nil uploader")
	}
}

func TestUpload_MissingReader(t *testing.T) {
	drv := &mockUploaderDriver{}
	u := NewUploader(drv, newTestCipher())

	_, err := u.Upload(context.Background(), Request{
		Path:       "/test.txt",
		Name:       "test.txt",
		ParentFid:  "parent123",
		PlainSize:  100,
		DataReader: nil,
	})

	if err == nil {
		t.Fatal("expected error for missing reader")
	}
}

func TestUpload_Basic(t *testing.T) {
	drv := &mockUploaderDriver{
		putFn: func(ctx context.Context, parentID, name string, size int64, body io.Reader) (drivers.Entry, error) {
			_ = parentID
			_ = name
			_ = size
			_, _ = io.ReadAll(body)
			return drivers.Entry{ID: "new_fid_123"}, nil
		},
	}

	u := NewUploader(drv, newTestCipher())

	content := []byte("test file content for upload")
	reader := io.NopCloser(bytes.NewReader(content))

	result, err := u.Upload(context.Background(), Request{
		Path:       "/upload_test.txt",
		Name:       "upload_test.txt",
		ParentFid:  "parent123",
		PlainSize:  int64(len(content)),
		DataReader: func() (io.ReadCloser, error) { return reader, nil },
	})
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	if result.Fid != "new_fid_123" {
		t.Errorf("expected fid 'new_fid_123', got %q", result.Fid)
	}
}
