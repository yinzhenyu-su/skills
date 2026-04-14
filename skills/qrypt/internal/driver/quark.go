package driver

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	QuarkBaseURL = "https://drive.quark.cn/1/clouddrive"
	QuarkV2URL   = "https://drive.quark.cn/api/v2"
	QuarkV2AltURL = "https://drive-api.quark.cn/api/v2"
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
func (d *QuarkDriver) isMgmtPath(path string) bool {
	return strings.HasPrefix(path, "/file/create_dir") ||
		strings.HasPrefix(path, "/file/delete") ||
		strings.HasPrefix(path, "/file/rename") ||
		strings.HasPrefix(path, "/file/move")
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

func (d *QuarkDriver) requestWithBase(method, baseURL, path string, query map[string]string, body interface{}, result interface{}) error {
	u, _ := url.Parse(baseURL + path)
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

// request 发起 HTTP 请求并解析响应
func (d *QuarkDriver) request(method, path string, query map[string]string, body interface{}, result interface{}) error {
	if !d.isMgmtPath(path) {
		return d.requestWithBase(method, QuarkBaseURL, path, query, body, result)
	}

	// 管理接口优先对齐 AList：先走 /1/clouddrive，再回退 /api/v2。
	bases := []string{QuarkBaseURL, QuarkV2URL, QuarkV2AltURL}
	var lastErr error
	for i, base := range bases {
		err := d.requestWithBase(method, base, path, query, body, result)
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

func (d *QuarkDriver) getOSSURL(pre *UpPreResp) (string, error) {
	host := pre.Data.UploadUrl
	if strings.HasPrefix(host, "http://") {
		host = host[7:]
	} else if strings.HasPrefix(host, "https://") {
		host = host[8:]
	}
	// 剥离路径
	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}
	return fmt.Sprintf("https://%s.%s/%s", pre.Data.Bucket, host, pre.Data.ObjKey), nil
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
		"format_type":     0,
	}
	var resp UpPreResp
	err := d.request(http.MethodPost, "/file/upload/pre", nil, data, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// UploadPart 封装了授权和上传
func (d *QuarkDriver) UploadPart(pre *UpPreResp, partNumber int, data []byte) (string, error) {
	dateStr := time.Now().UTC().Format(http.TimeFormat)

	// 与 Quark Web/AList 行为对齐：part 签名包含 x-oss-user-agent
	authMeta := fmt.Sprintf("PUT\n\napplication/octet-stream\n%s\nx-oss-date:%s\nx-oss-user-agent:aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit\n/%s/%s?partNumber=%d&uploadId=%s",
		dateStr, dateStr, pre.Data.Bucket, pre.Data.ObjKey, partNumber, pre.Data.UploadId)

	// 2. 获取 AuthKey
	authData := map[string]interface{}{
		"auth_info":   pre.Data.AuthInfo,
		"auth_meta":   authMeta,
		"task_id":     pre.Data.TaskId,
		"part_number": partNumber,
	}
	var authResp UpAuthResp
	err := d.request(http.MethodPost, "/file/upload/auth", nil, authData, &authResp)
	if err != nil {
		return "", err
	}

	// 3. 上传到 OSS
	u, _ := d.getOSSURL(pre)
	req, err := http.NewRequest(http.MethodPut, u, bytes.NewReader(data))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", authResp.Data.AuthKey)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("x-oss-date", dateStr)
	req.Header.Set("x-oss-user-agent", "aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit")
	req.Header.Set("Referer", QuarkReferer)
	req.Header.Set("User-Agent", QuarkUA)

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
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("oss put status: %d, host: %s, error: %s", resp.StatusCode, req.URL.Host, string(bodyBytes))
	}

	return resp.Header.Get("Etag"), nil
}

// UpdateHash 向夸克上报对象哈希，用于完成上传链路校验
func (d *QuarkDriver) UpdateHash(md5Hex, sha1Hex, taskID string) (bool, string, error) {
	data := map[string]interface{}{
		"md5":     md5Hex,
		"sha1":    sha1Hex,
		"task_id": taskID,
	}
	var resp HashResp
	err := d.request(http.MethodPost, "/file/update/hash", nil, data, &resp)
	if err != nil {
		return false, "", err
	}
	return resp.Data.Finish, resp.Data.Fid, nil
}

func encodeOSSCallback(callbackRaw json.RawMessage) (string, error) {
	if len(callbackRaw) == 0 {
		return "", errors.New("missing callback payload")
	}
	return base64.StdEncoding.EncodeToString(callbackRaw), nil
}

// UploadCommit 提交 multipart upload 结果到 OSS
func (d *QuarkDriver) UploadCommit(pre *UpPreResp, etags []string) error {
	// 1. 构建 XML body
	bodyBuilder := strings.Builder{}
	bodyBuilder.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<CompleteMultipartUpload>
`)
	for i, etag := range etags {
		bodyBuilder.WriteString(fmt.Sprintf(`<Part>
<PartNumber>%d</PartNumber>
<ETag>%s</ETag>
</Part>
`, i+1, etag))
	}
	bodyBuilder.WriteString("</CompleteMultipartUpload>")
	body := bodyBuilder.String()

	// 2. 计算 Content-MD5
	m := md5.New()
	m.Write([]byte(body))
	contentMd5 := base64.StdEncoding.EncodeToString(m.Sum(nil))

	// 3. 构建 auth_meta
	timeStr := time.Now().UTC().Format(http.TimeFormat)
	callbackBase64, err := encodeOSSCallback(pre.Data.Callback)
	if err != nil {
		return err
	}
	
	// 与 Quark Web/AList 行为对齐：commit 签名包含 x-oss-user-agent
	authMeta := fmt.Sprintf("POST\n%s\napplication/xml\n%s\nx-oss-callback:%s\nx-oss-date:%s\nx-oss-user-agent:aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit\n/%s/%s?uploadId=%s",
		contentMd5, timeStr, callbackBase64, timeStr, pre.Data.Bucket, pre.Data.ObjKey, pre.Data.UploadId)

	// 4. 获取 auth_key
	authData := map[string]interface{}{
		"auth_info": pre.Data.AuthInfo,
		"auth_meta": authMeta,
		"task_id":   pre.Data.TaskId,
	}
	var authResp UpAuthResp
	err = d.request(http.MethodPost, "/file/upload/auth", nil, authData, &authResp)
	if err != nil {
		return err
	}

	// 5. 发送 POST 到 OSS
	u, _ := d.getOSSURL(pre)
	req, err := http.NewRequest(http.MethodPost, u, strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", authResp.Data.AuthKey)
	req.Header.Set("Content-MD5", contentMd5)
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("x-oss-callback", callbackBase64)
	req.Header.Set("x-oss-date", timeStr)
	req.Header.Set("x-oss-user-agent", "aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit")
	req.Header.Set("Referer", QuarkReferer)
	req.Header.Set("User-Agent", QuarkUA)

	q := req.URL.Query()
	q.Set("uploadId", pre.Data.UploadId)
	req.URL.RawQuery = q.Encode()

	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("oss commit status: %d, error: %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
}

// UploadFinish 最终通知夸克
func (d *QuarkDriver) UploadFinish(pre *UpPreResp) error {
	data := map[string]interface{}{
		"obj_key": pre.Data.ObjKey,
		"task_id": pre.Data.TaskId,
	}
	return d.request(http.MethodPost, "/file/upload/finish", nil, data, nil)
}


// CreateDir 创建文件夹
func (d *QuarkDriver) CreateDir(pdirFid, name string) (string, error) {
	data := map[string]interface{}{
		"pdir_fid": pdirFid,
		"dir_name": name,
	}
	var resp struct {
		Resp
		Data struct {
			Fid string `json:"fid"`
		} `json:"data"`
	}
	// 注意：元数据操作通常使用 /api/v2 路径，这里根据调研结果尝试
	err := d.request(http.MethodPost, "/file/create_dir", nil, data, &resp)
	if err != nil {
		return "", err
	}
	if resp.Status >= 400 || resp.Code != 0 {
		return "", errors.New(resp.Message)
	}
	return resp.Data.Fid, nil
}

// Delete 删除文件或文件夹
func (d *QuarkDriver) Delete(fids []string) error {
	data := map[string]interface{}{
		"action_type":  1,
		"exclude_fids": []string{},
		"filelist":     fids,
	}
	var resp Resp
	err := d.request(http.MethodPost, "/file/delete", nil, data, &resp)
	if err != nil {
		return err
	}
	if resp.Status >= 400 || resp.Code != 0 {
		return errors.New(resp.Message)
	}
	return nil
}

// Rename 重命名文件或文件夹
func (d *QuarkDriver) Rename(fid, newName string) error {
	data := map[string]interface{}{
		"fid":       fid,
		"file_name": newName,
	}
	var resp Resp
	err := d.request(http.MethodPost, "/file/rename", nil, data, &resp)
	if err != nil {
		return err
	}
	if resp.Status >= 400 || resp.Code != 0 {
		return errors.New(resp.Message)
	}
	return nil
}

// Move 移动文件或文件夹
func (d *QuarkDriver) Move(fids []string, toPdirFid string) error {
	data := map[string]interface{}{
		"fids":        fids,
		"to_pdir_fid": toPdirFid,
	}
	var resp Resp
	err := d.request(http.MethodPost, "/file/move", nil, data, &resp)
	if err != nil {
		return err
	}
	if resp.Status >= 400 || resp.Code != 0 {
		return errors.New(resp.Message)
	}
	return nil
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
