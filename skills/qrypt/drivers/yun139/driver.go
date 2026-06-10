package yun139

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"

	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
)

type Yun139Driver struct {
	cl     *client
	rootID string
}

var (
	_ drivers.Driver   = (*Yun139Driver)(nil)
	_ drivers.Writer   = (*Yun139Driver)(nil)
	_ drivers.Uploader = (*Yun139Driver)(nil)
)

func init() {
	drivers.Register("yun139", drivers.DriverMeta{
		Ctor: func(params drivers.Params) (drivers.Driver, error) {
			auth := params["authorization"]
			if auth == "" {
				return nil, fmt.Errorf("missing authorization for yun139 driver")
			}
			return NewDriver(auth, params["root_id"]), nil
		},
		Params: []drivers.ParamSpec{
			{Key: "authorization", Required: true, Help: "139 authorization token"},
			{Key: "root_id", Help: "Remote root folder ID", Default: "/"},
		},
		RootKey:       "root_id",
		CredentialKey: "authorization",
	})
}

func NewDriver(authorization, rootID string) *Yun139Driver {
	return &Yun139Driver{
		cl:     newClient(authorization),
		rootID: rootID,
	}
}

func (d *Yun139Driver) Init(ctx context.Context) error {
	_, _, err := d.cl.decodeAuth()
	if err != nil {
		return fmt.Errorf("139 init: invalid authorization: %w", err)
	}
	if d.rootID == "" {
		d.rootID = "/"
	}
	if err := d.cl.ensurePersonalCloudHost(); err != nil {
		return fmt.Errorf("139 init: resolve personal cloud host: %w", err)
	}
	return nil
}

func (d *Yun139Driver) Drop(ctx context.Context) error { return nil }

func (d *Yun139Driver) List(ctx context.Context, parentID string) ([]drivers.Entry, error) {
	fileID := parentID
	if fileID == "" || fileID == "0" || fileID == "/" {
		fileID = d.rootID
	}

	var allEntries []drivers.Entry
	cursor := ""
	for {
		data := map[string]interface{}{
			"imageThumbnailStyleList": []string{"Small", "Large"},
			"orderBy":                 "updated_at",
			"orderDirection":          "DESC",
			"pageInfo": map[string]interface{}{
				"pageCursor": cursor,
				"pageSize":   100,
			},
			"parentFileId": fileID,
		}
		var resp personalListResp
		err := 	d.cl.personalPost("/file/list", data, &resp)
		if err != nil {
			return nil, fmt.Errorf("139 list: %w", err)
		}
		if !resp.Success {
			return nil, fmt.Errorf("139 list failed: %s", resp.Message)
		}
		allEntries = append(allEntries, toEntries(resp.Data.Items)...)
		cursor = resp.Data.NextPageCursor
		if cursor == "" {
			break
		}
	}
	return allEntries, nil
}

func (d *Yun139Driver) Read(ctx context.Context, entry drivers.Entry, offset, size int64) (io.ReadCloser, error) {
	url, err := d.getDownloadURL(entry.ID)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("139 read: %w", err)
	}
	if size > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+size-1))
	}

	resp, err := d.cl.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("139 read: download: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return nil, fmt.Errorf("139 read: status %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func (d *Yun139Driver) getDownloadURL(fileID string) (string, error) {
	data := map[string]interface{}{"fileId": fileID}
	var resp downloadResp
	err := d.cl.personalPost( "/file/getDownloadUrl", data, &resp)
	if err != nil {
		return "", fmt.Errorf("139 download url: %w", err)
	}
	if !resp.Success {
		return "", fmt.Errorf("139 download url failed: %s", resp.Message)
	}
	if resp.Data.CdnUrl != "" {
		return resp.Data.CdnUrl, nil
	}
	return resp.Data.Url, nil
}

func (d *Yun139Driver) Mkdir(ctx context.Context, parentID, name string) (drivers.Entry, error) {
	fileID := parentID
	if fileID == "" || fileID == "0" || fileID == "/" {
		fileID = d.rootID
	}
	data := map[string]interface{}{
		"parentFileId":   fileID,
		"name":           name,
		"description":    "",
		"type":           "folder",
		"fileRenameMode": "force_rename",
	}
	var resp createResp
	err := d.cl.personalPost( "/file/create", data, &resp)
	if err != nil {
		return drivers.Entry{}, fmt.Errorf("139 mkdir: %w", err)
	}
	if !resp.Success {
		return drivers.Entry{}, fmt.Errorf("139 mkdir failed: %s", resp.Message)
	}
	return drivers.Entry{ID: resp.Data.FileId, Name: resp.Data.Name, IsDir: true}, nil
}

func (d *Yun139Driver) Move(ctx context.Context, entry drivers.Entry, dstParentID string) error {
	srcFileID := entry.ID
	if srcFileID == "" || srcFileID == "0" || srcFileID == "/" {
		srcFileID = d.rootID
	}
	toParentID := dstParentID
	if toParentID == "" || toParentID == "0" || toParentID == "/" {
		toParentID = d.rootID
	}
	data := map[string]interface{}{
		"fileIds":        []string{srcFileID},
		"toParentFileId": toParentID,
	}
	var resp baseResp
	err := d.cl.personalPost( "/file/batchMove", data, &resp)
	if err != nil {
		return fmt.Errorf("139 move: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("139 move failed: %s", resp.Message)
	}
	return nil
}

func (d *Yun139Driver) Rename(ctx context.Context, entry drivers.Entry, newName string) error {
	srcFileID := entry.ID
	if srcFileID == "" || srcFileID == "0" || srcFileID == "/" {
		srcFileID = d.rootID
	}
	data := map[string]interface{}{
		"fileId":      srcFileID,
		"name":        newName,
		"description": "",
	}
	var resp baseResp
	err := d.cl.personalPost( "/file/update", data, &resp)
	if err != nil {
		return fmt.Errorf("139 rename: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("139 rename failed: %s", resp.Message)
	}
	return nil
}

func (d *Yun139Driver) Remove(ctx context.Context, entry drivers.Entry) error {
	data := map[string]interface{}{
		"fileIds": []string{entry.ID},
	}
	var resp baseResp
	err := d.cl.personalPost( "/recyclebin/batchTrash", data, &resp)
	if err != nil {
		return fmt.Errorf("139 remove: %w", err)
	}
	if !resp.Success {
		return fmt.Errorf("139 remove failed: %s", resp.Message)
	}
	return nil
}

func (d *Yun139Driver) Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (drivers.Entry, error) {
	fileID := parentID
	if fileID == "" || fileID == "0" || fileID == "/" {
		fileID = d.rootID
	}

	partSize := d.calcPartSize(size)
	part := size / partSize
	if size%partSize > 0 {
		part++
	} else if part == 0 {
		part = 1
	}

	// Read body into memory to compute SHA256 (required by 139 API).
	// For large files this could be optimised with a temp file, but the
	// FUSE upload path already stages on disk so this is acceptable.
	allData, err := io.ReadAll(body)
	if err != nil {
		return drivers.Entry{}, fmt.Errorf("139 upload: read body: %w", err)
	}

	// Build part infos.
	type partInfo struct {
		PartNumber int64 `json:"partNumber"`
		PartSize   int64 `json:"partSize"`
		ParallelHashCtx struct {
			PartOffset int64 `json:"partOffset"`
		} `json:"parallelHashCtx"`
	}
	partInfos := make([]partInfo, 0, part)
	for i := int64(0); i < part; i++ {
		start := i * partSize
		byteSize := size - start
		if byteSize > partSize {
			byteSize = partSize
		}
		partInfos = append(partInfos, partInfo{
			PartNumber: i + 1,
			PartSize:   byteSize,
			ParallelHashCtx: struct {
				PartOffset int64 `json:"partOffset"`
			}{PartOffset: start},
		})
	}

	// For the first 100 partInfos only (the rest will be fetched via
	// /file/getUploadUrl after the create call).
	firstPartInfos := partInfos
	if len(firstPartInfos) > 100 {
		firstPartInfos = firstPartInfos[:100]
	}

	// Compute SHA256 of the full content (for dedup).
	sha256Hex := fmt.Sprintf("%X", sha256.Sum256(allData))

	createData := map[string]interface{}{
		"contentHash":          sha256Hex,
		"contentHashAlgorithm": "SHA256",
		"contentType":          "application/octet-stream",
		"parallelUpload":       false,
		"partInfos":            firstPartInfos,
		"size":                 size,
		"parentFileId":         fileID,
		"name":                 name,
		"type":                 "file",
		"fileRenameMode":       "auto_rename",
	}
	var createResp personalUploadResp
	err = d.cl.personalPost("/file/create", createData, &createResp)
	if err != nil {
		return drivers.Entry{}, fmt.Errorf("139 upload create: %w", err)
	}
	if !createResp.Success {
		return drivers.Entry{}, fmt.Errorf("139 upload create failed: %s", createResp.Message)
	}

	logging.L.Debugf("139 upload create: fileId=%s exist=%v rapid=%v parts=%d uploadId=%s",
		createResp.Data.FileId, createResp.Data.Exist, createResp.Data.RapidUpload,
		len(createResp.Data.PartInfos), createResp.Data.UploadId)

	// File already exists on server (duplicate).
	if createResp.Data.Exist {
		return drivers.Entry{
			ID:   createResp.Data.FileId,
			Name: name,
			Size: size,
		}, nil
	}

	// Gather all upload URLs.
	type uploadPart struct {
		partNumber int
		uploadURL  string
	}
	var uploadParts []uploadPart

	if createResp.Data.PartInfos != nil {
		for _, p := range createResp.Data.PartInfos {
			uploadParts = append(uploadParts, uploadPart{
				partNumber: p.PartNumber,
				uploadURL:  p.UploadUrl,
			})
		}
	}

	// Fetch upload URLs for parts beyond the first 100.
	for i := 101; i <= len(partInfos); i += 100 {
		end := i + 100
		if end > len(partInfos) {
			end = len(partInfos)
		}
		batchPartInfos := partInfos[i-1 : end]
		moreData := map[string]interface{}{
			"fileId":   createResp.Data.FileId,
			"uploadId": createResp.Data.UploadId,
			"partInfos": batchPartInfos,
			"commonAccountInfo": map[string]interface{}{
				"account":     d.cl.getAccount(),
				"accountType": 1,
			},
		}
		var moreResp personalUploadUrlResp
		err = d.cl.personalPost("/file/getUploadUrl", moreData, &moreResp)
		if err != nil {
			return drivers.Entry{}, fmt.Errorf("139 upload get urls: %w", err)
		}
		if !moreResp.Success {
			return drivers.Entry{}, fmt.Errorf("139 upload get urls failed: %s", moreResp.Message)
		}
		for _, p := range moreResp.Data.PartInfos {
			uploadParts = append(uploadParts, uploadPart{
				partNumber: p.PartNumber,
				uploadURL:  p.UploadUrl,
			})
		}
	}

	// Upload parts.
	for _, up := range uploadParts {
		start := int64(up.partNumber-1) * partSize
		end := start + partSize
		if end > size {
			end = size
		}
		partData := allData[start:end]

		req, err := http.NewRequestWithContext(ctx, http.MethodPut, up.uploadURL, bytes.NewReader(partData))
		if err != nil {
			return drivers.Entry{}, fmt.Errorf("139 upload part %d: %w", up.partNumber, err)
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Content-Length", fmt.Sprint(len(partData)))
		req.Header.Set("Origin", "https://yun.139.com")
		req.Header.Set("Referer", "https://yun.139.com/")
		resp, err := d.cl.httpClient.Do(req)
		if err != nil {
			return drivers.Entry{}, fmt.Errorf("139 upload part %d: %w", up.partNumber, err)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return drivers.Entry{}, fmt.Errorf("139 upload part %d: status %d body=%s", up.partNumber, resp.StatusCode, string(bodyBytes))
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	// Finalize: commit the uploaded parts to create the file.
	completeData := map[string]interface{}{
		"contentHash":          sha256Hex,
		"contentHashAlgorithm": "SHA256",
		"fileId":               createResp.Data.FileId,
		"uploadId":             createResp.Data.UploadId,
	}
	var completeResp baseResp
	err = d.cl.personalPost("/file/complete", completeData, &completeResp)
	if err != nil {
		return drivers.Entry{}, fmt.Errorf("139 upload complete: %w", err)
	}
	if !completeResp.Success {
		return drivers.Entry{}, fmt.Errorf("139 upload complete failed: %s", completeResp.Message)
	}

	return drivers.Entry{ID: createResp.Data.FileId, Name: name, Size: size}, nil
}

func (d *Yun139Driver) calcPartSize(fileSize int64) int64 {
	switch {
	case fileSize <= 0:
		return 4 * 1024 * 1024
	case fileSize <= 100*1024*1024:
		return 4 * 1024 * 1024
	case fileSize <= 500*1024*1024:
		return 10 * 1024 * 1024
	case fileSize <= 1*1024*1024*1024:
		return 20 * 1024 * 1024
	default:
		return 50 * 1024 * 1024
	}
}

func (d *Yun139Driver) ResolvePath(ctx context.Context, path string) (string, error) {
	if path == "" || path == "/" {
		return d.rootID, nil
	}
	return d.rootID, nil
}
