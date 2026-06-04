package quark

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var (
	baseURL   = "https://drive.quark.cn/1/clouddrive"
	v2URL     = "https://drive.quark.cn/api/v2"
	v2AltURL  = "https://drive.quark.cn/api/v2"
	referer   = "https://pan.quark.cn"
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) quark-cloud-drive/2.5.20 Chrome/100.0.4896.160 Electron/18.3.5.4-b478491100 Safari/537.36 Channel/pckk_other_ch"
)

const (
	httpMaxRetries = 3
	ossMaxRetries  = 3
)

type client struct {
	httpClient     *http.Client
	downloadClient *http.Client
	cookie         string
	mu             sync.RWMutex
	sem            chan struct{}
	mgmtSem        chan struct{}
	metaSem        chan struct{}
}

func newClient(cookie string) *client {
	return &client{
		httpClient:     newHTTPClient(30 * time.Second),
		downloadClient: newHTTPClient(0),
		cookie:         cookie,
		sem:            make(chan struct{}, 200),
		mgmtSem:        make(chan struct{}, 500),
		metaSem:        make(chan struct{}, 500),
	}
}

func newHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	c := &http.Client{Transport: transport}
	if timeout > 0 {
		c.Timeout = timeout
	}
	return c
}

func (c *client) cookieValue() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cookie
}

func (c *client) setCookie(cookie string) {
	c.mu.Lock()
	c.cookie = cookie
	c.mu.Unlock()
}

func (c *client) updateCookie(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	parts := strings.Split(c.cookie, "; ")
	found := false
	for i, p := range parts {
		if strings.HasPrefix(p, key+"=") {
			parts[i] = key + "=" + value
			found = true
			break
		}
	}
	if !found {
		parts = append(parts, key+"="+value)
	}
	c.cookie = strings.Join(parts, "; ")
}

func isMgmtPath(path string) bool {
	return strings.HasPrefix(path, "/file/delete") ||
		strings.HasPrefix(path, "/file/rename") ||
		strings.HasPrefix(path, "/file/move") ||
		strings.HasPrefix(path, "/file/upload/commit") ||
		strings.HasPrefix(path, "/file/upload/finish")
}

func isMetaPath(path string) bool {
	return strings.HasPrefix(path, "/file/list") ||
		strings.HasPrefix(path, "/file/sort") ||
		strings.HasPrefix(path, "/file/search")
}

func isRetryableHTTPError(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := errors.AsType[net.Error](err); ok {
		return true
	}
	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "tls handshake") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "connection refused")
}

func isRetryableHTTPStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

func retryBackoff(attempt int) time.Duration {
	base := time.Duration(500<<uint(attempt)) * time.Millisecond
	jitter := float64(75+(attempt*7)%50) / 100.0
	return time.Duration(float64(base) * jitter)
}

func shouldRetryWithAltBase(err error) bool {
	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such host") || strings.Contains(msg, "lookup ")
}

func tryNextMgmtBase(err error) bool {
	if shouldRetryWithAltBase(err) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "(Status 404)") || strings.Contains(msg, "(Status 405)")
}

func (c *client) doRequest(method, baseURL, path string, query map[string]string, body interface{}, result interface{}) error {
	u, _ := url.Parse(baseURL + path)
	q := u.Query()
	for k, v := range query {
		q.Set(k, v)
	}
	q.Set("pr", "ucpro")
	q.Set("fr", "pc")
	u.RawQuery = q.Encode()

	var sem chan struct{}
	if isMgmtPath(path) {
		sem = c.mgmtSem
	} else if isMetaPath(path) {
		sem = c.metaSem
	} else {
		sem = c.sem
	}
	sem <- struct{}{}
	defer func() { <-sem }()

	cookie := c.cookieValue()

	for attempt := 0; attempt <= httpMaxRetries; attempt++ {
		var bodyReader io.Reader
		if body != nil {
			jsonBody, _ := json.Marshal(body)
			bodyReader = bytes.NewReader(jsonBody)
		}

		req, err := http.NewRequest(method, u.String(), bodyReader)
		if err != nil {
			return fmt.Errorf("create request failed: %w", err)
		}

		req.Header.Set("Cookie", cookie)
		req.Header.Set("Origin", "https://pan.quark.cn")
		req.Header.Set("Referer", referer)
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "application/json, text/plain, */*")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if attempt < httpMaxRetries && isRetryableHTTPError(err) {
				time.Sleep(retryBackoff(attempt))
				continue
			}
			return fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		if isRetryableHTTPStatus(resp.StatusCode) && attempt < httpMaxRetries {
			time.Sleep(retryBackoff(attempt))
			continue
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read response failed: %w", err)
		}

		for _, ck := range resp.Cookies() {
			if ck.Name == "__puus" {
				c.updateCookie("__puus", ck.Value)
			}
			if ck.Name == "__pus" {
				c.updateCookie("__pus", ck.Value)
			}
		}

		if result != nil {
			if err := json.Unmarshal(bodyBytes, result); err != nil {
				return fmt.Errorf("parse response failed: %w", err)
			}
		}

		if resp.StatusCode >= 400 {
			return fmt.Errorf("API Error (Status %d): %s", resp.StatusCode, string(bodyBytes))
		}

		return nil
	}

	return fmt.Errorf("max retries exceeded")
}

// request issues an HTTP request to the primary base URL, falling back to the
// management alternative base for retryable management-path errors.
func (c *client) request(method, path string, query map[string]string, body, result interface{}) error {
	bases := []string{baseURL}
	if isMgmtPath(path) {
		bases = append(bases, v2URL)
	}

	var lastErr error
	for _, b := range bases {
		err := c.doRequest(method, b, path, query, body, result)
		if err == nil {
			return nil
		}
		if isMgmtPath(path) && tryNextMgmtBase(err) {
			lastErr = err
			continue
		}
		return err
	}
	return lastErr
}

func (c *client) doDownload(req *http.Request) (*http.Response, error) {
	req.Header.Set("Cookie", c.cookieValue())
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", referer)
	return c.downloadClient.Do(req)
}
