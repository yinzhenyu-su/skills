package upload

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
)

type mockUploaderDriver struct {
	qrypt.Driver
	putFn func(ctx context.Context, parentID, name string, size int64, body io.Reader) (qrypt.Entry, error)
}

func (m *mockUploaderDriver) Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (qrypt.Entry, error) {
	return m.putFn(ctx, parentID, name, size, body)
}

type ucipherAdapter struct {
	inner *qrypt.RcloneCipher
}

func (a *ucipherAdapter) EncryptSegment(plain string) string { return a.inner.EncryptSegment(plain) }
func (a *ucipherAdapter) DecryptSegment(cipher string) (string, error) { return a.inner.DecryptSegment(cipher) }
func (a *ucipherAdapter) EncryptBlock(plaintext []byte, blockIndex uint64, nonce [qrypt.FileNonceSize]byte) ([]byte, error) {
	return a.inner.EncryptBlock(plaintext, blockIndex, nonce)
}
func (a *ucipherAdapter) DecryptBlock(ciphertext []byte, blockIndex uint64, nonce [qrypt.FileNonceSize]byte) ([]byte, error) {
	return a.inner.DecryptBlock(ciphertext, blockIndex, nonce)
}
func (a *ucipherAdapter) EncryptedSize(plainSize int64) int64 { return a.inner.EncryptedSize(plainSize) }
func (a *ucipherAdapter) DecryptedSize(cipherSize int64) (int64, error) { return a.inner.DecryptedSize(cipherSize) }
func (a *ucipherAdapter) GenerateRandomNonce() ([qrypt.FileNonceSize]byte, error) { return a.inner.GenerateRandomNonce() }

func newTestCipher() qrypt.Cipher {
	c, _ := qrypt.NewRcloneCipher("password", "")
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
	var capturedParent string
	var capturedSize int64
	var capturedBody []byte

	drv := &mockUploaderDriver{
		putFn: func(ctx context.Context, parentID, name string, size int64, body io.Reader) (qrypt.Entry, error) {
			capturedParent = parentID
			_ = name
			capturedSize = size
			capturedBody, _ = io.ReadAll(body)
			return qrypt.Entry{ID: "new_fid_123"}, nil
		},
	}

	u := NewUploader(drv, newTestCipher())

	content := []byte("hello world this is a test")
	result, err := u.Upload(context.Background(), Request{
		Path:       "/test.txt",
		Name:       "test.txt",
		ParentFid:  "parent123",
		PlainSize:  int64(len(content)),
		DataReader: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(content)), nil },
	})

	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	if result.Fid != "new_fid_123" {
		t.Fatalf("expected fid new_fid_123, got %s", result.Fid)
	}
	if result.EncryptedSize <= 0 {
		t.Fatal("expected positive encrypted size")
	}
	if capturedParent != "parent123" {
		t.Fatalf("expected parent parent123, got %s", capturedParent)
	}
	if capturedBody == nil || len(capturedBody) == 0 {
		t.Fatal("expected non-empty captured body")
	}

	t.Logf("plain=%d, encrypted=%d, captured=%d", len(content), capturedSize, len(capturedBody))
}

func TestUpload_DefaultNonce(t *testing.T) {
	drv := &mockUploaderDriver{
		putFn: func(ctx context.Context, parentID, name string, size int64, body io.Reader) (qrypt.Entry, error) {
			return qrypt.Entry{ID: "fid"}, nil
		},
	}

	u := NewUploader(drv, newTestCipher())

	content := []byte("test data with nonce generation")
	result, err := u.Upload(context.Background(), Request{
		Path:       "/nonce_test.bin",
		Name:       "nonce_test.bin",
		ParentFid:  "parent",
		PlainSize:  int64(len(content)),
		Nonce:      [24]byte{},
		DataReader: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(content)), nil },
	})

	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	if isZeroNonceLocal(result.Nonce) {
		t.Fatal("expected nonce to be generated (non-zero)")
	}
}

func TestUpload_CustomNonce(t *testing.T) {
	drv := &mockUploaderDriver{
		putFn: func(ctx context.Context, parentID, name string, size int64, body io.Reader) (qrypt.Entry, error) {
			return qrypt.Entry{ID: "fid"}, nil
		},
	}

	u := NewUploader(drv, newTestCipher())

	customNonce := [24]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24}
	content := []byte("test with explicit nonce")
	result, err := u.Upload(context.Background(), Request{
		Path:       "/custom_nonce.bin",
		Name:       "custom_nonce.bin",
		ParentFid:  "parent",
		PlainSize:  int64(len(content)),
		Nonce:      customNonce,
		DataReader: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(content)), nil },
	})

	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	if result.Nonce != customNonce {
		t.Fatal("expected custom nonce to be preserved")
	}
}

func TestUpload_ProgressFn(t *testing.T) {
	var progressCalls []int
	drv := &mockUploaderDriver{
		putFn: func(ctx context.Context, parentID, name string, size int64, body io.Reader) (qrypt.Entry, error) {
			return qrypt.Entry{ID: "fid"}, nil
		},
	}

	u := NewUploader(drv, newTestCipher())

	content := []byte("progress tracking test")
	_, err := u.Upload(context.Background(), Request{
		Path:       "/progress.bin",
		Name:       "progress.bin",
		ParentFid:  "parent",
		PlainSize:  int64(len(content)),
		DataReader: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(content)), nil },
		ProgressFn: func(partNumber int) { progressCalls = append(progressCalls, partNumber) },
	})

	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
}

func TestPartRetryBackoff(t *testing.T) {
	t.Skip("partRetryBackoff was unused dead code; removed in cleanup")
}

func TestIsZeroNonce(t *testing.T) {
	if !isZeroNonceLocal([24]byte{}) {
		t.Fatal("expected zero nonce to be detected")
	}
	if isZeroNonceLocal([24]byte{1}) {
		t.Fatal("expected non-zero nonce to be detected")
	}
	if isZeroNonceLocal([24]byte{23: 1}) {
		t.Fatal("expected non-zero nonce to be detected")
	}
}

// isZeroNonceLocal is a test helper; the production version lives in qrypt
// (unexported). Keep this minimal — it mirrors the kernel implementation.
func isZeroNonceLocal(n [24]byte) bool {
	for _, b := range n {
		if b != 0 {
			return false
		}
	}
	return true
}

func BenchmarkUploadThroughput(b *testing.B) {
	drv := &mockUploaderDriver{
		putFn: func(ctx context.Context, parentID, name string, size int64, body io.Reader) (qrypt.Entry, error) {
			_, _ = io.Copy(io.Discard, body)
			return qrypt.Entry{ID: "bench_fid"}, nil
		},
	}

	data := make([]byte, 64*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	ciph, _ := qrypt.NewRcloneCipher("password", "")
	u := NewUploader(drv, &ucipherAdapter{inner: ciph})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := u.Upload(context.Background(), Request{
			Path:       "/bench.bin",
			Name:       "bench.bin",
			ParentFid:  "parent",
			PlainSize:  int64(len(data)),
			DataReader: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil },
		})
		if err != nil {
			b.Fatalf("Upload failed: %v", err)
		}
	}
}
