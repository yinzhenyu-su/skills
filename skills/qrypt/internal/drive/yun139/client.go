package yun139

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

var baseURL = "https://api.139.com/api"

type client struct {
	httpClient    *http.Client
	authorization string
	account       string
	mu            sync.RWMutex
	tokenExpiry   time.Time
}

func newClient(authorization string) *client {
	return &client{
		httpClient:    &http.Client{Timeout: 60 * time.Second},
		authorization: authorization,
	}
}

func (c *client) getAuthorization() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.authorization
}

func (c *client) setAuthorization(auth string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.authorization = auth
}

func (c *client) decodeAuth() (account, token string, err error) {
	raw := c.getAuthorization()
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return "", "", fmt.Errorf("decode auth: %w", err)
	}
	parts := strings.Split(string(decoded), ":")
	if len(parts) < 3 {
		return "", "", fmt.Errorf("invalid auth format")
	}
	return parts[1], parts[2], nil
}

func (c *client) refreshToken() error {
	account, token, err := c.decodeAuth()
	if err != nil {
		return err
	}

	body := fmt.Sprintf("<root><token>%s</token><account>%s</account><clienttype>656</clienttype></root>", token, account)
	req, err := http.NewRequest(http.MethodPost, "https://aas.caiyun.feixin.10086.cn:443/tellin/authTokenRefresh.do", strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var refreshResp struct {
		Return string `xml:"return"`
		Desc   string `xml:"desc"`
		Token  string `xml:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&refreshResp); err != nil {
		return fmt.Errorf("token refresh failed: %s", err)
	}
	if refreshResp.Return != "0" {
		return fmt.Errorf("token refresh failed: %s", refreshResp.Desc)
	}

	raw, _ := base64.StdEncoding.DecodeString(c.getAuthorization())
	parts := strings.Split(string(raw), ":")
	if len(parts) >= 3 {
		parts[2] = refreshResp.Token
	}
	newAuth := base64.StdEncoding.EncodeToString([]byte(strings.Join(parts, ":")))
	c.setAuthorization(newAuth)
	return nil
}

func (c *client) doRequest(method, path string, body interface{}, result interface{}) error {
	u := baseURL + path
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = strings.NewReader(string(data))
	}

	req, err := http.NewRequest(method, u, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.getAuthorization())
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("parse response: %w", err)
		}
	}
	return nil
}
