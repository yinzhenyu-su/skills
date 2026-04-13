package driver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	QuarkBaseURL = "https://drive.quark.cn/1/clouddrive"
	QuarkReferer = "https://pan.quark.cn"
	QuarkUA      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) quark-cloud-drive/2.5.20 Chrome/100.0.4896.160 Electron/18.3.5.4-b478491100 Safari/537.36 Channel/pckk_other_ch"
)

// QuarkDriver 封装了与夸克网盘 API 的交互
type QuarkDriver struct {
	client   *http.Client
	cookie   string
	urlCache sync.Map // fid -> cachedURL
}

type cachedURL struct {
	url    string
	expiry time.Time
}

// NewQuarkDriver 创建一个新的驱动实例
func NewQuarkDriver(cookie string) *QuarkDriver {
	return &QuarkDriver{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		cookie: cookie,
	}
}

// request 发起 HTTP 请求并解析响应
func (d *QuarkDriver) request(method, path string, query map[string]string, body interface{}, result interface{}) error {
	u, _ := url.Parse(QuarkBaseURL + path)
	q := u.Query()
	for k, v := range query {
		q.Set(k, v)
	}
	q.Set("pr", "ucpro")
	q.Set("fr", "pc")
	u.RawQuery = q.Encode()

	var bodyReader io.Reader
	if body != nil {
		jsonBody, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, u.String(), bodyReader)
	if err != nil {
		return err
	}

	d.setHeaders(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 自动更新 Cookie (__puus)
	for _, c := range resp.Cookies() {
		if c.Name == "__puus" {
			d.cookie = d.updateCookie(d.cookie, "__puus", c.Value)
		}
	}

	if resp.StatusCode >= 400 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API Error (Status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}

	return nil
}

func (d *QuarkDriver) updateCookie(old, key, value string) string {
	parts := strings.Split(old, "; ")
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
	return strings.Join(parts, "; ")
}

// GetDownloadURL 获取文件的下载地址 (带缓存逻辑)
func (d *QuarkDriver) GetDownloadURL(fid string) (string, error) {
	// 1. 检查缓存
	if val, ok := d.urlCache.Load(fid); ok {
		c := val.(cachedURL)
		if time.Now().Before(c.expiry) {
			return c.url, nil
		}
	}

	// 2. 缓存失效或不存在，发起请求
	data := map[string]interface{}{
		"fids": []string{fid},
	}
	var resp DownResp
	err := d.request(http.MethodPost, "/file/download", nil, data, &resp)
	if err != nil {
		return "", err
	}

	if resp.Status >= 400 || resp.Code != 0 {
		return "", errors.New(resp.Message)
	}

	if len(resp.Data) == 0 {
		return "", errors.New("no download url found")
	}

	url := resp.Data[0].DownloadUrl
	// 3. 存入缓存 (有效期 10 分钟)
	d.urlCache.Store(fid, cachedURL{
		url:    url,
		expiry: time.Now().Add(10 * time.Minute),
	})

	return url, nil
}

// DownloadChunk 下载指定范围的文件分块
func (d *QuarkDriver) DownloadChunk(downloadURL string, start, end int64) (io.ReadCloser, error) {
	req, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	d.setHeaders(req)

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return nil, fmt.Errorf("download error: status %d", resp.StatusCode)
	}

	return resp.Body, nil
}

// UploadPre 预上传请求
func (d *QuarkDriver) UploadPre(fileName, parentFid string, size int64) (*UpPreResp, error) {
	now := time.Now().UnixMilli()
	data := map[string]interface{}{
		"ccp_hash_update": true,
		"file_name":       fileName,
		"l_created_at":    now,
		"l_updated_at":    now,
		"pdir_fid":        parentFid,
		"size":            size,
	}
	var resp UpPreResp
	err := d.request(http.MethodPost, "/file/upload/pre", nil, data, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// UploadAuth 获取授权 Key
func (d *QuarkDriver) UploadAuth(pre *UpPreResp, partNumber int, method, contentType, authMeta string) (string, error) {
	data := map[string]interface{}{
		"auth_info": pre.Data.AuthInfo,
		"auth_meta": authMeta,
		"task_id":   pre.Data.TaskId,
	}
	var resp UpAuthResp
	err := d.request(http.MethodPost, "/file/upload/auth", nil, data, &resp)
	if err != nil {
		return "", err
	}
	return resp.Data.AuthKey, nil
}

// UploadPart 上传单个分块
func (d *QuarkDriver) UploadPart(pre *UpPreResp, partNumber int, data io.Reader, authKey, contentType, dateStr string) (string, error) {
	u := fmt.Sprintf("https://%s.%s/%s", pre.Data.Bucket, pre.Data.UploadUrl[7:], pre.Data.ObjKey)
	req, err := http.NewRequest(http.MethodPut, u, data)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", authKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-oss-date", dateStr)
	req.Header.Set("Referer", QuarkReferer)
	req.Header.Set("x-oss-user-agent", "aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit")

	q := req.URL.Query()
	q.Set("partNumber", strconv.Itoa(partNumber))
	q.Set("uploadId", pre.Data.UploadId)
	req.URL.RawQuery = q.Encode()

	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("up status: %d", resp.StatusCode)
	}

	return resp.Header.Get("Etag"), nil
}

// UploadCommit 提交结果
func (d *QuarkDriver) UploadCommit(pre *UpPreResp, etags []string) error {
	return nil
}

// UploadFinish 完成
func (d *QuarkDriver) UploadFinish(pre *UpPreResp) error {
	data := map[string]interface{}{
		"obj_key": pre.Data.ObjKey,
		"task_id": pre.Data.TaskId,
	}
	return d.request(http.MethodPost, "/file/upload/finish", nil, data, nil)
}

// ListFiles 获取文件列表
func (d *QuarkDriver) ListFiles(parentFid string) ([]File, error) {
	var files []File
	page := 1
	size := 100

	for {
		var resp SortResp
		err := d.request(http.MethodGet, "/file/sort", map[string]string{
			"pdir_fid":             parentFid,
			"_size":                strconv.Itoa(size),
			"_page":                strconv.Itoa(page),
			"_fetch_total":         "1",
			"fetch_all_file":       "1",
			"fetch_risk_file_name": "1",
		}, nil, &resp)
		if err != nil {
			return nil, err
		}

		if resp.Status >= 400 || resp.Code != 0 {
			return nil, errors.New(resp.Message)
		}

		files = append(files, resp.Data.List...)

		if page*size >= resp.Metadata.Total {
			break
		}
		page++
	}

	return files, nil
}

// FindChildByName 查找子节点
func (d *QuarkDriver) FindChildByName(parentFid, name string) (string, error) {
	files, err := d.ListFiles(parentFid)
	if err != nil {
		return "", err
	}

	fmt.Printf("Debug: Searching for '%s' in FID '%s'. Found %d items:\n", name, parentFid, len(files))
	for _, f := range files {
		if f.FileName == name {
			return f.Fid, nil
		}
	}

	return "", fmt.Errorf("child not found: %s", name)
}

// ResolvePath 解析路径
func (d *QuarkDriver) ResolvePath(encryptedPath string) (string, error) {
	segments := strings.Split(strings.Trim(encryptedPath, "/"), "/")
	currentFid := "0"

	for _, seg := range segments {
		if seg == "" {
			continue
		}
		fid, err := d.FindChildByName(currentFid, seg)
		if err != nil {
			return "", err
		}
		currentFid = fid
	}

	return currentFid, nil
}

// Auth 验证
func (d *QuarkDriver) Auth() error {
	if d.cookie == "" {
		return errors.New("cookie is required")
	}

	var resp SortResp
	err := d.request(http.MethodGet, "/file/sort", map[string]string{
		"pdir_fid": "0",
		"_size":    "1",
	}, nil, &resp)
	if err != nil {
		return err
	}

	if resp.Status >= 400 || resp.Code != 0 {
		return errors.New(resp.Message)
	}

	return nil
}

// setHeaders
func (d *QuarkDriver) setHeaders(req *http.Request) {
	req.Header.Set("Cookie", d.cookie)
	req.Header.Set("User-Agent", QuarkUA)
	req.Header.Set("Referer", QuarkReferer)
	req.Header.Set("Accept", "application/json, text/plain, */*")
}
