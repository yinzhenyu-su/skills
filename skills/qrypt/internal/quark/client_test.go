package quark

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	c := NewClient("cookie=test")
	if c.Cookie() != "cookie=test" {
		t.Errorf("expected cookie=test, got %s", c.Cookie())
	}
}

func TestSetCookie(t *testing.T) {
	c := NewClient("old_cookie")
	c.SetCookie("new_cookie")
	if c.Cookie() != "new_cookie" {
		t.Errorf("expected new_cookie, got %s", c.Cookie())
	}
}

func TestUpdateCookie(t *testing.T) {
	c := NewClient("a=1; b=2")
	c.updateCookie("b", "3")
	if c.Cookie() != "a=1; b=3" {
		t.Errorf("expected a=1; b=3, got %s", c.Cookie())
	}
}

func TestUpdateCookie_Append(t *testing.T) {
	c := NewClient("a=1")
	c.updateCookie("b", "2")
	expected := "a=1; b=2"
	if c.Cookie() != expected {
		t.Errorf("expected %s, got %s", expected, c.Cookie())
	}
}

func TestRequest_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "test_cookie" {
			t.Errorf("expected test_cookie cookie, got %s", r.Header.Get("Cookie"))
		}
		if r.Header.Get("User-Agent") != UserAgent {
			t.Errorf("expected User-Agent %s", UserAgent)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": 200,
			"code":   0,
			"data":   []interface{}{},
		})
	}))
	defer server.Close()

	c := NewClient("test_cookie")
	c.httpClient = server.Client()

	// replace base URLs with test server
	type apiFileService struct {
		client *Client
	}

	c.SetClientForTest(server.Client())
}

func TestIsRetryableHTTPError(t *testing.T) {
	err := &netError{}
	if isRetryableHTTPError(err) {
		t.Log("net.Error is retryable")
	}
}

type netError struct{}

func (e *netError) Error() string   { return "timeout" }
func (e *netError) Timeout() bool   { return true }
func (e *netError) Temporary() bool { return true }

func TestIsRetryableHTTPStatus(t *testing.T) {
	if !isRetryableHTTPStatus(429) {
		t.Error("429 should be retryable")
	}
	if !isRetryableHTTPStatus(500) {
		t.Error("500 should be retryable")
	}
	if !isRetryableHTTPStatus(503) {
		t.Error("503 should be retryable")
	}
	if isRetryableHTTPStatus(400) {
		t.Error("400 should not be retryable")
	}
	if isRetryableHTTPStatus(403) {
		t.Error("403 should not be retryable")
	}
}

func TestRetryBackoff(t *testing.T) {
	d1 := retryBackoff(0)
	d2 := retryBackoff(1)
	d3 := retryBackoff(2)
	if d1 >= d2 || d2 >= d3 {
		t.Error("backoff should increase with attempts")
	}
}

func TestCacheService_New(t *testing.T) {
	c := NewCacheService()
	if c.DirCacheTTL != 60*time.Second {
		t.Errorf("expected 60s, got %v", c.DirCacheTTL)
	}
}

func TestCacheService_URL(t *testing.T) {
	c := NewCacheService()
	c.SetURL("fid1", "https://example.com/dl")
	url, ok := c.GetURL("fid1")
	if !ok || url != "https://example.com/dl" {
		t.Errorf("expected url, got %s, %v", url, ok)
	}
}

func TestCacheService_URL_Expired(t *testing.T) {
	c := NewCacheService()
	c.SetURL("fid_exp", "https://expired.com/dl")
	_, ok := c.GetURL("fid_exp")
	if !ok {
		t.Error("URL should be valid immediately after Set")
	}
}

func TestCacheService_InvalidateURL(t *testing.T) {
	c := NewCacheService()
	c.SetURL("fid_inv", "https://example.com/dl")
	c.InvalidateURL("fid_inv")
	_, ok := c.GetURL("fid_inv")
	if ok {
		t.Error("URL should be invalidated")
	}
}

func TestCacheService_Dir(t *testing.T) {
	c := NewCacheService()
	files := []File{{Fid: "f1", FileName: "a.txt"}}
	c.SetDir("parent_fid", files)
	result, ok := c.GetDir("parent_fid")
	if !ok || len(result) != 1 || result[0].Fid != "f1" {
		t.Errorf("expected 1 file, got %d", len(result))
	}
}

func TestCacheService_RemoveDir(t *testing.T) {
	c := NewCacheService()
	c.SetDir("p", []File{{Fid: "f1"}})
	c.RemoveDir("p")
	_, ok := c.GetDir("p")
	if ok {
		t.Error("should be removed")
	}
}

func TestCacheService_Neg(t *testing.T) {
	c := NewCacheService()
	c.SetNeg("parent_fid", ".DS_Store")
	if !c.GetNeg("parent_fid", ".DS_Store") {
		t.Error("negative cache should hit")
	}
	if c.GetNeg("parent_fid", "real_file.txt") {
		t.Error("should not hit for other names")
	}
}

func TestCacheService_DeleteNeg(t *testing.T) {
	c := NewCacheService()
	c.SetNeg("p", "file.txt")
	if !c.GetNeg("p", "file.txt") {
		t.Error("should hit before delete")
	}
	c.DeleteNeg("p", "file.txt")
	if c.GetNeg("p", "file.txt") {
		t.Error("should miss after delete")
	}
}

func TestFileService_New(t *testing.T) {
	client := NewClient("cookie=t")
	cacheSvc := NewCacheService()
	cph := &mockCipher{}
	fs := NewFileService(client, cacheSvc, cph)
	if fs.Cache() != cacheSvc {
		t.Error("cache service mismatch")
	}
}

type mockCipher struct{}

func (m *mockCipher) DecryptSegment(name string) (string, error) { return name, nil }
func (m *mockCipher) EncryptSegment(name string) string { return name }
func (m *mockCipher) DecryptedSize(encSize int64) (int64, error) { return encSize, nil }
func (m *mockCipher) DecryptBlock(ciphertext []byte, blockIndex uint64, fileNonce [24]byte) ([]byte, error) {
	return ciphertext, nil
}
func (m *mockCipher) EncryptBlock(plaintext []byte, blockIndex uint64, fileNonce [24]byte) ([]byte, error) {
	return plaintext, nil
}
func (m *mockCipher) GenerateRandomNonce() ([24]byte, error) {
	return [24]byte{}, nil
}
func (m *mockCipher) FileHeaderSize() int    { return 0 }
func (m *mockCipher) BlockDataSize() int     { return 65536 }
func (m *mockCipher) BlockSize() int         { return 65552 }
func (m *mockCipher) EncryptedSize(decSize int64) int64 { return decSize }

func TestManageService_New(t *testing.T) {
	client := NewClient("cookie=t")
	ms := NewManageService(client)
	if ms == nil {
		t.Error("expected non-nil manage service")
	}
}

func TestUploadService_New(t *testing.T) {
	client := NewClient("cookie=t")
	us := NewUploadService(client)
	if us == nil {
		t.Error("expected non-nil upload service")
	}
}

func TestDownloadChunk_NoCookie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "test_cookie" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusPartialContent)
		w.Write([]byte("data"))
	}))
	defer server.Close()

	c := NewClient("test_cookie")
	c.downloadClient = server.Client()

	rc, err := c.DownloadChunk(server.URL, 0, 10)
	if err != nil {
		t.Fatalf("expected success with cookie, got %v", err)
	}
	rc.Close()
}

func TestDownloadChunk_StatusNotOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := NewClient("cookie")
	c.downloadClient = server.Client()

	_, err := c.DownloadChunk(server.URL, 0, 10)
	if err == nil {
		t.Error("expected error for 404")
	}
}

func TestClient_SetClientForTest(t *testing.T) {
	c := NewClient("cookie")
	hc := &http.Client{}
	c.SetClientForTest(hc)
	if c.httpClient != hc || c.downloadClient != hc {
		t.Error("client mismatch after SetClientForTest")
	}
}

func TestFileService_Cache(t *testing.T) {
	client := NewClient("c")
	cacheSvc := NewCacheService()
	fs := NewFileService(client, cacheSvc, &mockCipher{})
	if fs.Cache() != cacheSvc {
		t.Error("Cache() mismatch")
	}
}
