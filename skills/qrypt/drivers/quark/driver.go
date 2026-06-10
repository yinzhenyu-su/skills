package quark

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yinzhenyu/skills/qrypt/cipher"
	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
)

type QuarkDriver struct {
	cl       *client
	cache    *cacheManager
	cookie   string
	rootPath string
	cipher   cipher.Cipher
}

func (d *QuarkDriver) SetCipher(c cipher.Cipher) {
	d.cipher = c
}

var (
	_ drivers.Driver       = (*QuarkDriver)(nil)
	_ drivers.Writer       = (*QuarkDriver)(nil)
	_ drivers.Uploader     = (*QuarkDriver)(nil)
	_ drivers.CipherSetter = (*QuarkDriver)(nil)
)

func init() {
	drivers.Register("quark", drivers.DriverMeta{
		Ctor: func(params drivers.Params) (drivers.Driver, error) {
			cookie := params["cookie"]
			if cookie == "" {
				return nil, fmt.Errorf("missing cookie for quark driver")
			}
			dirCacheTTL, _ := time.ParseDuration(params["dir_cache_ttl"])
			if dirCacheTTL <= 0 {
				dirCacheTTL = 60 * time.Second
			}
			return NewDriver(cookie, params["root_path"], dirCacheTTL), nil
		},
		Params: []drivers.ParamSpec{
			{Key: "cookie", Required: true, Help: "Quark cookie string"},
			{Key: "root_path", Help: "Remote root folder path", Default: "/"},
			{Key: "dir_cache_ttl", Help: "Directory cache TTL (e.g. 30s, 5m)", Default: "60s"},
		},
		RootKey:       "root_path",
		CredentialKey: "cookie",
	})
}

func NewDriver(cookie, rootPath string, dirCacheTTL time.Duration) *QuarkDriver {
	return &QuarkDriver{
		cl:       newClient(cookie),
		cache:    newCacheManager(dirCacheTTL),
		cookie:   cookie,
		rootPath: rootPath,
	}
}



func (d *QuarkDriver) Init(ctx context.Context) error {
	if d.cookie == "" {
		return fmt.Errorf("cookie is required")
	}
	var resp sortResp
	err := d.cl.request(http.MethodGet, "/file/sort", map[string]string{
		"pdir_fid": "0",
		"_size":    "1",
	}, nil, &resp)
	if err != nil {
		return fmt.Errorf("validate cookie: %w", err)
	}
	if rerr := apiError(resp.resp); rerr != nil {
		return rerr
	}
	return nil
}

func (d *QuarkDriver) Drop(ctx context.Context) error {
	return nil
}

func (d *QuarkDriver) List(ctx context.Context, parentID string) ([]drivers.Entry, error) {
	if files, ok := d.cache.getDir(parentID); ok {
		return toEntries(files), nil
	}

	size := 100
	var firstResp sortResp
	err := d.cl.request(http.MethodGet, "/file/sort", map[string]string{
		"pdir_fid":             parentID,
		"_size":                strconv.Itoa(size),
		"_page":                "1",
		"_fetch_total":         "1",
		"fetch_all_file":       "1",
		"fetch_risk_file_name": "1",
	}, nil, &firstResp)
	if err != nil {
		return nil, fmt.Errorf("list: %w", err)
	}
	if rerr := apiError(firstResp.resp); rerr != nil {
		return nil, rerr
	}

	total := firstResp.Metadata.Total
	allFiles := make([]file, total)
	copy(allFiles, firstResp.Data.List)

	if total > size {
		totalPages := (total + size - 1) / size
		var wg sync.WaitGroup
		var errOnce sync.Once
		var lastErr error

		for p := 2; p <= totalPages; p++ {
			wg.Add(1)
			go func(page int) {
				defer wg.Done()
				var resp sortResp
				err := d.cl.request(http.MethodGet, "/file/sort", map[string]string{
					"pdir_fid":             parentID,
					"_size":                strconv.Itoa(size),
					"_page":                strconv.Itoa(page),
					"fetch_all_file":       "1",
					"fetch_risk_file_name": "1",
				}, nil, &resp)
				if err != nil {
					errOnce.Do(func() { lastErr = err })
					return
				}
				if rerr := apiError(resp.resp); rerr != nil {
					errOnce.Do(func() { lastErr = rerr })
					return
				}
				offset := (page - 1) * size
				if offset < len(allFiles) {
					copy(allFiles[offset:], resp.Data.List)
				}
			}(p)
		}
		wg.Wait()
		if lastErr != nil {
			return nil, lastErr
		}
	}

	d.cache.setDir(parentID, allFiles)
	return toEntries(allFiles), nil
}

func (d *QuarkDriver) Read(ctx context.Context, entry drivers.Entry, offset, size int64) (io.ReadCloser, error) {
	fid := entry.ID
	downloadURL, err := d.getDownloadURL(fid)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("read: create request: %w", err)
	}

	if size > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+size-1))
	}

	resp, err := d.cl.doDownload(req)
	if err != nil {
		d.cache.invalidateURL(fid)
		return nil, fmt.Errorf("read: download: %w", err)
	}

	if resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		d.cache.invalidateURL(fid)
		return d.Read(ctx, entry, offset, size)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return nil, fmt.Errorf("read: unexpected status %d", resp.StatusCode)
	}

	return resp.Body, nil
}

func (d *QuarkDriver) getDownloadURL(fid string) (string, error) {
	if url, ok := d.cache.getURL(fid); ok {
		return url, nil
	}

	data := map[string]interface{}{
		"fids": []string{fid},
	}
	var resp downResp
	err := d.cl.request(http.MethodPost, "/file/download", nil, data, &resp)
	if err != nil {
		return "", fmt.Errorf("get download url: %w", err)
	}
	if rerr := apiError(resp.resp); rerr != nil {
		return "", rerr
	}
	if len(resp.Data) == 0 {
		return "", fmt.Errorf("no download url found")
	}
	url := resp.Data[0].DownloadUrl
	d.cache.setURL(fid, url)
	return url, nil
}

func apiError(resp resp) error {
	if resp.Status < 400 && resp.Code == 0 {
		return nil
	}
	switch resp.Code {
	case 23001, 23004:
		return drivers.ErrNotFound
	case 23008:
		return drivers.ErrDirAlreadyExists
	}
	return fmt.Errorf("api error: status=%d code=%d msg=%s", resp.Status, resp.Code, resp.Message)
}

func (d *QuarkDriver) Mkdir(ctx context.Context, parentID, name string) (drivers.Entry, error) {
	data := map[string]interface{}{
		"pdir_fid":      parentID,
		"file_name":     name,
		"dir_path":      "",
		"dir_init_lock": false,
	}
	var resp createDirResp
	err := d.cl.request(http.MethodPost, "/file", nil, data, &resp)
	if err != nil {
		return drivers.Entry{}, fmt.Errorf("mkdir: %w", err)
	}
	if rerr := apiError(resp.resp); rerr != nil {
		return drivers.Entry{}, rerr
	}
	d.cache.removeDir(parentID)
	return drivers.Entry{
		ID:    resp.Data.Fid,
		Name:  name,
		IsDir: true,
	}, nil
}

func (d *QuarkDriver) Move(ctx context.Context, entry drivers.Entry, dstParentID string) error {
	data := map[string]interface{}{
		"filelist":     []string{entry.ID},
		"to_pdir_fid":  dstParentID,
		"action_type":  1,
		"exclude_fids": []string{},
	}
	var resp resp
	err := d.cl.request(http.MethodPost, "/file/move", nil, data, &resp)
	if err != nil {
		return fmt.Errorf("move: %w", err)
	}
	if rerr := apiError(resp); rerr != nil {
		return rerr
	}
	if entry.ParentID != "" {
		d.cache.removeDir(entry.ParentID)
	}
	d.cache.removeDir(dstParentID)
	return nil
}

func (d *QuarkDriver) Rename(ctx context.Context, entry drivers.Entry, newName string) error {
	data := map[string]interface{}{
		"fid":       entry.ID,
		"file_name": newName,
	}
	var resp resp
	err := d.cl.request(http.MethodPost, "/file/rename", nil, data, &resp)
	if err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	if rerr := apiError(resp); rerr != nil {
		return rerr
	}
	if entry.ParentID != "" {
		d.cache.removeDir(entry.ParentID)
	}
	return nil
}

func (d *QuarkDriver) Remove(ctx context.Context, entry drivers.Entry) error {
	data := map[string]interface{}{
		"action_type":  1,
		"exclude_fids": []string{},
		"filelist":     []string{entry.ID},
	}
	var resp resp
	err := d.cl.request(http.MethodPost, "/file/delete", nil, data, &resp)
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	if rerr := apiError(resp); rerr != nil {
		return rerr
	}
	return nil
}

func (d *QuarkDriver) Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (drivers.Entry, error) {
	d.deleteExistingFileByName(parentID, name)

	mtime := time.Now()
	if mt, ok := drivers.MtimeFromContext(ctx); ok {
		mtime = mt
	}
	preData := map[string]interface{}{
		"ccp_hash_update": true,
		"file_name":       name,
		"l_created_at":    mtime.UnixMilli(),
		"l_updated_at":    mtime.UnixMilli(),
		"pdir_fid":        parentID,
		"size":            size,
		"format_type":     0,
	}
	var preResp upPreResp
	err := d.cl.request(http.MethodPost, "/file/upload/pre", nil, preData, &preResp)
	if err != nil {
		return drivers.Entry{}, fmt.Errorf("upload pre: %w", err)
	}
	if rerr := apiError(preResp.resp); rerr != nil {
		return drivers.Entry{}, rerr
	}

	if preResp.Data.Finish && preResp.Data.Fid != "" {
		finishData := map[string]interface{}{
			"fid":       preResp.Data.Fid,
			"obj_key":   preResp.Data.ObjKey,
			"bucket":    preResp.Data.Bucket,
			"task_id":   preResp.Data.TaskId,
			"upload_id": preResp.Data.UploadId,
		}
		d.cl.request(http.MethodPost, "/file/upload/finish", nil, finishData, nil)
		return drivers.Entry{ID: preResp.Data.Fid, Name: name, Size: size}, nil
	}

	partSize := preResp.Metadata.PartSize
	if partSize <= 0 {
		partSize = 4 * 1024 * 1024
	}

	// Streaming upload: read and upload parts one at a time to avoid
	// loading the entire file into memory.
	md5Hash := md5.New()
	sha1Hash := sha1.New()
	hashWriter := io.MultiWriter(md5Hash, sha1Hash)
	teeReader := io.TeeReader(body, hashWriter)

	var etags []string
	buf := make([]byte, partSize)
	totalRead := int64(0)
	partNumber := 1

	for {
		n, readErr := io.ReadFull(teeReader, buf)
		if n > 0 {
			etag, err := d.uploadPart(&preResp, partNumber, buf[:n])
			if err != nil {
				return drivers.Entry{}, fmt.Errorf("upload part %d: %w", partNumber, err)
			}
			etags = append(etags, etag)
			totalRead += int64(n)
			partNumber++
		}
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
		if readErr != nil {
			return drivers.Entry{}, fmt.Errorf("upload: read body: %w", readErr)
		}
	}

	// Empty file: upload one empty part so Quark creates the file entry.
	if totalRead == 0 {
		etag, err := d.uploadPart(&preResp, 1, []byte{})
		if err != nil {
			return drivers.Entry{}, fmt.Errorf("upload part 1: %w", err)
		}
		etags = append(etags, etag)
	}

	encSize := totalRead
	md5Hex := fmt.Sprintf("%X", md5Hash.Sum(nil))
	sha1Hex := fmt.Sprintf("%X", sha1Hash.Sum(nil))

	hashData := map[string]interface{}{
		"md5":     md5Hex,
		"sha1":    sha1Hex,
		"task_id": preResp.Data.TaskId,
	}
	var hashResp hashResp
	err = d.cl.request(http.MethodPost, "/file/update/hash", nil, hashData, &hashResp)
	if err != nil {
		return drivers.Entry{}, fmt.Errorf("upload hash: %w", err)
	}

	if hashResp.Data.Finish {
		if hashResp.Data.Fid != "" {
			preResp.Data.Fid = hashResp.Data.Fid
		}
		d.uploadFinish(preResp.Data.Fid, preResp.Data.ObjKey, preResp.Data.TaskId)
		return drivers.Entry{
			ID:   preResp.Data.Fid,
			Name: name,
			Size: encSize,
		}, nil
	}

	if err := d.ossComplete(&preResp, etags); err != nil {
		return drivers.Entry{}, fmt.Errorf("upload complete: %w", err)
	}

	d.uploadFinish(preResp.Data.Fid, preResp.Data.ObjKey, preResp.Data.TaskId)
	return drivers.Entry{
		ID:   preResp.Data.Fid,
		Name: name,
		Size: encSize,
	}, nil
}

func (d *QuarkDriver) uploadFinish(fid, objKey, taskID string) {
	finishData := map[string]interface{}{
		"obj_key": objKey,
		"task_id": taskID,
	}
	d.cl.request(http.MethodPost, "/file/upload/finish", nil, finishData, nil)
}

// ossComplete sends OSS CompleteMultipartUpload directly to Alibaba OSS
// (NOT a Quark API call). /file/upload/commit does not exist — do not use it.
func (d *QuarkDriver) ossComplete(pre *upPreResp, etags []string) error {
	if len(etags) == 0 {
		return nil
	}

	var xmlBody strings.Builder
	xmlBody.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<CompleteMultipartUpload>`)
	for i, etag := range etags {
		xmlBody.WriteString(fmt.Sprintf(`
<Part>
<PartNumber>%d</PartNumber>
<ETag>%s</ETag>
</Part>`, i+1, etag))
	}
	xmlBody.WriteString(`
</CompleteMultipartUpload>`)
	body := xmlBody.String()

	m := md5.New()
	m.Write([]byte(body))
	contentMd5 := base64.StdEncoding.EncodeToString(m.Sum(nil))

	for attempt := 0; attempt <= ossMaxRetries; attempt++ {
		timeStr := time.Now().UTC().Format(http.TimeFormat)
		callbackB64 := base64.StdEncoding.EncodeToString([]byte(pre.Data.Callback))
		authMeta := fmt.Sprintf("POST\n%s\napplication/xml\n%s\nx-oss-callback:%s\nx-oss-date:%s\nx-oss-user-agent:aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit\n/%s/%s?uploadId=%s",
			contentMd5, timeStr, callbackB64, timeStr, pre.Data.Bucket, pre.Data.ObjKey, pre.Data.UploadId)

		authData := map[string]interface{}{
			"auth_info": pre.Data.AuthInfo,
			"auth_meta": authMeta,
			"task_id":   pre.Data.TaskId,
		}
		var authResp upAuthResp
		err := d.cl.request(http.MethodPost, "/file/upload/auth", nil, authData, &authResp)
		if err != nil {
			if attempt < ossMaxRetries {
				time.Sleep(retryBackoff(attempt))
				continue
			}
			return err
		}

		u := getOSSURL(pre)
		q := "?uploadId=" + pre.Data.UploadId
		req, err := http.NewRequest(http.MethodPost, u+q, strings.NewReader(body))
		if err != nil {
			return err
		}

		req.Header.Set("Authorization", authResp.Data.AuthKey)
		req.Header.Set("Content-MD5", contentMd5)
		req.Header.Set("Content-Type", "application/xml")
		req.Header.Set("x-oss-callback", callbackB64)
		req.Header.Set("x-oss-date", timeStr)
		req.Header.Set("x-oss-user-agent", "aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit")
		req.Header.Set("Referer", referer)
		req.Header.Set("User-Agent", userAgent)

		resp, err := d.cl.httpClient.Do(req)
		if err != nil {
			if attempt < ossMaxRetries {
				time.Sleep(retryBackoff(attempt))
				continue
			}
			return fmt.Errorf("oss complete: %w", err)
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		if attempt < ossMaxRetries {
			time.Sleep(retryBackoff(attempt))
			continue
		}
		return fmt.Errorf("oss complete status %d", resp.StatusCode)
	}
	return nil
}

func (d *QuarkDriver) uploadPart(pre *upPreResp, partNumber int, data []byte) (string, error) {
	for attempt := 0; attempt <= ossMaxRetries; attempt++ {
		dateStr := time.Now().UTC().Format(http.TimeFormat)
		authMeta := fmt.Sprintf("PUT\n\napplication/octet-stream\n%s\nx-oss-date:%s\nx-oss-user-agent:aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit\n/%s/%s?partNumber=%d&uploadId=%s",
			dateStr, dateStr, pre.Data.Bucket, pre.Data.ObjKey, partNumber, pre.Data.UploadId)

		authData := map[string]interface{}{
			"auth_info":   pre.Data.AuthInfo,
			"auth_meta":   authMeta,
			"task_id":     pre.Data.TaskId,
			"part_number": partNumber,
		}
		var authResp upAuthResp
		err := d.cl.request(http.MethodPost, "/file/upload/auth", nil, authData, &authResp)
		if err != nil {
			if attempt < ossMaxRetries {
				time.Sleep(retryBackoff(attempt))
				continue
			}
			return "", err
		}

		u := getOSSURL(pre)
		q := "?partNumber=" + strconv.Itoa(partNumber) + "&uploadId=" + pre.Data.UploadId
		req, err := http.NewRequest(http.MethodPut, u+q, bytes.NewReader(data))
		if err != nil {
			return "", err
		}

		req.Header.Set("Authorization", authResp.Data.AuthKey)
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("x-oss-date", dateStr)
		req.Header.Set("x-oss-user-agent", "aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit")
		req.Header.Set("Referer", "https://pan.quark.cn")

		resp, err := d.cl.httpClient.Do(req)
		if err != nil {
			if attempt < ossMaxRetries {
				time.Sleep(retryBackoff(attempt))
				continue
			}
			return "", fmt.Errorf("upload part %d http: %w", partNumber, err)
		}
		etag := resp.Header.Get("Etag")
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return etag, nil
		}
		if attempt < ossMaxRetries {
			time.Sleep(retryBackoff(attempt))
			continue
		}
		return "", fmt.Errorf("upload part %d status %d", partNumber, resp.StatusCode)
	}
	return "", nil
}

func getOSSURL(pre *upPreResp) string {
	host := pre.Data.UploadUrl
	if strings.HasPrefix(host, "http://") {
		host = host[7:]
	} else if strings.HasPrefix(host, "https://") {
		host = host[8:]
	}
	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}
	return fmt.Sprintf("https://%s.%s/%s", pre.Data.Bucket, host, pre.Data.ObjKey)
}

func (d *QuarkDriver) deleteExistingFileByName(parentFid, name string) {
	files, err := d.List(context.Background(), parentFid)
	if err != nil {
		logging.L.Warnf("deleteExistingFileByName: list parent %s: %v", parentFid, err)
		return
	}
	for _, f := range files {
		if f.Name == name && !f.IsDir {
			_ = d.Remove(context.Background(), f)
			logging.L.Infof("deleteExistingFileByName: removed existing %s (%s)", name, f.ID)
			return
		}
	}
}

func (d *QuarkDriver) ResolvePath(ctx context.Context, path string) (string, error) {
	fullPath := path
	if d.rootPath != "" && d.rootPath != "/" {
		trimmedPath := strings.TrimLeft(path, "/")
		trimmedRoot := strings.TrimLeft(d.rootPath, "/")
		if !strings.HasPrefix(trimmedPath, trimmedRoot) {
			if trimmedPath == "" {
				fullPath = d.rootPath
			} else {
				fullPath = strings.TrimRight(d.rootPath, "/") + "/" + trimmedPath
			}
		}
	}
	segments := strings.Split(strings.Trim(fullPath, "/"), "/")
	currentFid := "0"
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		entries, err := d.List(ctx, currentFid)
		if err != nil {
			return "", err
		}
		found := false
		encSeg := ""
		if d.cipher != nil {
			encSeg = d.cipher.EncryptSegment(seg)
		}
		for _, e := range entries {
			if e.Name == seg || (encSeg != "" && strings.EqualFold(e.Name, encSeg)) {
				if e.IsDir {
					currentFid = e.ID
					found = true
					break
				}
				if !found {
					currentFid = e.ID
					found = true
				}
			}
		}
		if !found {
			return "", fmt.Errorf("child not found: %s", seg)
		}
	}
	return currentFid, nil
}
