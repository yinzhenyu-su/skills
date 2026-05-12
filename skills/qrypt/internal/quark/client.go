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

	"github.com/yinzhenyu/skills/qrypt/internal/log"
)

var (
	BaseURL   = "https://drive.quark.cn/1/clouddrive"
	V2URL     = "https://drive.quark.cn/api/v2"
	V2AltURL  = "https://drive.quark.cn/api/v2"
	Referer   = "https://pan.quark.cn"
	UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) quark-cloud-drive/2.5.20 Chrome/100.0.4896.160 Electron/18.3.5.4-b478491100 Safari/537.36 Channel/pckk_other_ch"
)

const (
	httpMaxRetries = 3
	ossMaxRetries  = 3
)

type Client struct {
	httpClient    *http.Client
	downloadClient *http.Client
	cookie        string
	mu            sync.RWMutex
	sem           chan struct{}
	mgmtSem       chan struct{}
	metaSem       chan struct{}
}

func NewClient(cookie string) *Client {
	return &Client{
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

func (c *Client) SetCookie(cookie string) {
	c.mu.Lock()
	c.cookie = cookie
	c.mu.Unlock()
}

func (c *Client) Cookie() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cookie
}

func (c *Client) updateCookie(key, value string) {
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

func (c *Client) isMgmtPath(path string) bool {
	return strings.HasPrefix(path, "/file/delete") ||
		strings.HasPrefix(path, "/file/rename") ||
		strings.HasPrefix(path, "/file/move")
}

func (c *Client) isMetaPath(path string) bool {
	return strings.HasPrefix(path, "/file/list") ||
		strings.HasPrefix(path, "/file/sort") ||
		strings.HasPrefix(path, "/file/search")
}

func isRetryableHTTPError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
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
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such host") || strings.Contains(msg, "lookup ")
}

func shouldTryNextMgmtBase(err error) bool {
	if shouldRetryWithAltBase(err) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "API Error (Status 404)") || strings.Contains(msg, "API Error (Status 405)")
}

func (c *Client) doRequest(method, baseURL, path string, query map[string]string, body interface{}, result interface{}) error {
	u, _ := url.Parse(baseURL + path)
	q := u.Query()
	for k, v := range query {
		q.Set(k, v)
	}
	q.Set("pr", "ucpro")
	q.Set("fr", "pc")
	u.RawQuery = q.Encode()

	var sem chan struct{}
	if c.isMgmtPath(path) {
		sem = c.mgmtSem
	} else if c.isMetaPath(path) {
		sem = c.metaSem
	} else {
		sem = c.sem
	}
	sem <- struct{}{}
	defer func() { <-sem }()

	cookie := c.Cookie()

	for attempt := 0; attempt <= httpMaxRetries; attempt++ {
		var bodyReader io.Reader
		if body != nil {
			jsonBody, _ := json.Marshal(body)
			bodyReader = bytes.NewReader(jsonBody)
		}

		req, err := http.NewRequest(method, u.String(), bodyReader)
		if err != nil {
			return err
		}
		req.Header.Set("Cookie", cookie)
		req.Header.Set("User-Agent", UserAgent)
		req.Header.Set("Referer", Referer)
		req.Header.Set("Accept", "application/json, text/plain, */*")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if attempt < httpMaxRetries && isRetryableHTTPError(err) {
				log.L.Debugf("HTTP retry %d/%d: retry for %s: %v\n", attempt+1, httpMaxRetries, path, err)
				time.Sleep(retryBackoff(attempt))
				continue
			}
			return err
		}

		for _, ck := range resp.Cookies() {
			if ck.Name == "__puus" {
				log.L.Infof("Updated cookie __puus\n")
				c.updateCookie("__puus", ck.Value)
			}
		}

		if resp.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if attempt < httpMaxRetries && isRetryableHTTPStatus(resp.StatusCode) {
				log.L.Debugf("HTTP retry %d/%d: retry for %s status=%d\n", attempt+1, httpMaxRetries, path, resp.StatusCode)
				time.Sleep(retryBackoff(attempt))
				continue
			}
			return fmt.Errorf("API Error (Status %d): %s", resp.StatusCode, string(bodyBytes))
		}

		if result != nil {
			err = json.NewDecoder(resp.Body).Decode(result)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		return nil
	}
	return nil
}

func (c *Client) Request(method, path string, query map[string]string, body interface{}, result interface{}) error {
	if !c.isMgmtPath(path) {
		return c.doRequest(method, BaseURL, path, query, body, result)
	}
	bases := []string{BaseURL, V2URL, V2AltURL}
	var lastErr error
	for i, base := range bases {
		err := c.doRequest(method, base, path, query, body, result)
		if err == nil {
			return nil
		}
		lastErr = err
		if i == len(bases)-1 || !shouldTryNextMgmtBase(err) {
			break
		}
	}
	return lastErr
}

func (c *Client) OSSPut(url string, headers map[string]string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPut, url, body)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.httpClient.Do(req)
}

func (c *Client) OSSPost(url string, headers map[string]string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.httpClient.Do(req)
}

func (c *Client) DownloadChunk(downloadURL string, start, end int64) (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Referer", Referer)
	if cookie := c.Cookie(); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := c.downloadClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return nil, fmt.Errorf("download error: status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func (c *Client) SetClientForTest(hc *http.Client) {
	c.httpClient = hc
	c.downloadClient = hc
}
