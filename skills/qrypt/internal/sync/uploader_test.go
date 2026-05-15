package sync

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
)

type mockDriver struct {
	drive.Driver
	putFn func(ctx context.Context, parentID, name string, size int64, body io.Reader) (drive.Entry, error)
}

func (m *mockDriver) Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (drive.Entry, error) {
	return m.putFn(ctx, parentID, name, size, body)
}

func TestNewUploader(t *testing.T) {
	drv := &mockDriver{}
	c, _ := crypt.NewRcloneCipher("password", "")
	u := NewUploader(drv, c)
	if u == nil {
		t.Fatal("expected non-nil uploader")
	}
}

func TestUpload_MissingReader(t *testing.T) {
	drv := &mockDriver{}
	c, _ := crypt.NewRcloneCipher("password", "")
	u := NewUploader(drv, c)

	_, err := u.Upload(Request{
		Path:       "/test.txt",
		Name:       "test.txt",
		ParentFid:  "0",
		PlainSize:  100,
		DataReader: nil,
	})
	if err == nil {
		t.Fatal("expected error for missing data reader")
	}
}

func TestUpload_Success(t *testing.T) {
	drv := &mockDriver{
		putFn: func(ctx context.Context, parentID, name string, size int64, body io.Reader) (drive.Entry, error) {
			return drive.Entry{ID: "new_fid_123", Name: name}, nil
		},
	}
	c, _ := crypt.NewRcloneCipher("password", "")
	u := NewUploader(drv, c)

	result, err := u.Upload(Request{
		Path:      "/test.txt",
		Name:      "test.txt",
		ParentFid: "0",
		PlainSize: 100,
		DataReader: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(make([]byte, 100))), nil
		},
	})
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	if result.Fid != "new_fid_123" {
		t.Errorf("expected new_fid_123, got %s", result.Fid)
	}
	if result.EncryptedSize <= 100 {
		t.Errorf("expected encrypted size > 100, got %d", result.EncryptedSize)
	}
}

func TestUpload_WithNonce(t *testing.T) {
	drv := &mockDriver{
		putFn: func(ctx context.Context, parentID, name string, size int64, body io.Reader) (drive.Entry, error) {
			return drive.Entry{ID: "fid_nonce", Name: name}, nil
		},
	}
	c, _ := crypt.NewRcloneCipher("password", "")
	u := NewUploader(drv, c)

	var nonce [24]byte
	copy(nonce[:], []byte("provided_nonce_1234567890"))

	result, err := u.Upload(Request{
		Path:      "/nonce.txt",
		Name:      "nonce.txt",
		ParentFid: "0",
		PlainSize: 50,
		Nonce:     nonce,
		DataReader: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(make([]byte, 50))), nil
		},
	})
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	if result.Nonce != nonce {
		t.Errorf("nonce not preserved")
	}
}

func TestUpload_DriverPutError(t *testing.T) {
	drv := &mockDriver{
		putFn: func(ctx context.Context, parentID, name string, size int64, body io.Reader) (drive.Entry, error) {
			return drive.Entry{}, fmt.Errorf("network error")
		},
	}
	c, _ := crypt.NewRcloneCipher("password", "")
	u := NewUploader(drv, c)

	_, err := u.Upload(Request{
		Path:      "/fail.txt",
		Name:      "fail.txt",
		ParentFid: "0",
		PlainSize: 10,
		DataReader: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader([]byte("data"))), nil
		},
	})
	if err == nil {
		t.Fatal("expected error from driver Put")
	}
}

func TestIsZeroNonce(t *testing.T) {
	tests := []struct {
		name     string
		nonce    [24]byte
		expected bool
	}{
		{"all zeros", [24]byte{}, true},
		{"non-zero first byte", func() (n [24]byte) { n[0] = 1; return }(), false},
		{"non-zero last byte", func() (n [24]byte) { n[23] = 1; return }(), false},
		{"non-zero middle byte", func() (n [24]byte) { n[12] = 0xFF; return }(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isZeroNonce(tt.nonce)
			if result != tt.expected {
				t.Errorf("isZeroNonce(%v) = %v, want %v", tt.nonce, result, tt.expected)
			}
		})
	}
}

func TestPartRetryBackoff(t *testing.T) {
	d0 := partRetryBackoff(0)
	d1 := partRetryBackoff(1)
	d2 := partRetryBackoff(2)

	if d0 <= 0 {
		t.Error("backoff(0) should be > 0")
	}
	if d1 <= d0 {
		t.Error("backoff(1) should be > backoff(0)")
	}
	if d2 <= d1 {
		t.Error("backoff(2) should be > backoff(1)")
	}

	results := make(map[time.Duration]bool)
	for i := 0; i < 20; i++ {
		d := partRetryBackoff(0)
		results[d] = true
	}
	if len(results) < 2 {
		t.Logf("jitter produced only %d unique values out of 20 samples", len(results))
	}
}

func TestPartRetryBackoff_Range(t *testing.T) {
	for attempt := 0; attempt < 5; attempt++ {
		d := partRetryBackoff(attempt)
		min := time.Duration(200<<uint(attempt)) * time.Millisecond
		if d < time.Duration(float64(min)*0.5) || d > time.Duration(float64(min)*1.5) {
			t.Errorf("backoff(%d) = %v seems out of range (base=%v)", attempt, d, min)
		}
	}
}
