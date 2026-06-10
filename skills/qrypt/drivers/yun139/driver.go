package yun139

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/yinzhenyu/skills/qrypt/drivers"
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
		err := d.cl.doRequest(http.MethodPost, "/file/list", data, &resp)
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
	err := d.cl.doRequest(http.MethodPost, "/file/getDownloadUrl", data, &resp)
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
	err := d.cl.doRequest(http.MethodPost, "/file/create", data, &resp)
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
	err := d.cl.doRequest(http.MethodPost, "/file/batchMove", data, &resp)
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
	err := d.cl.doRequest(http.MethodPost, "/file/update", data, &resp)
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
	err := d.cl.doRequest(http.MethodPost, "/recyclebin/batchTrash", data, &resp)
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
	totalParts := int((size + partSize - 1) / partSize)
	if totalParts == 0 {
		totalParts = 1
	}

	initData := map[string]interface{}{
		"parentFileId": fileID,
		"fileName":     name,
		"fileSize":     size,
		"partCount":    totalParts,
		"partSize":     partSize,
	}
	var preResp uploadPreResp
	err := d.cl.doRequest(http.MethodPost, "/file/upload/init", initData, &preResp)
	if err != nil {
		return drivers.Entry{}, fmt.Errorf("139 upload init: %w", err)
	}
	if !preResp.Success {
		return drivers.Entry{}, fmt.Errorf("139 upload init failed: %s", preResp.Message)
	}

	// Streaming upload: read and upload parts one at a time.
	buf := make([]byte, partSize)
	for i := 0; i < totalParts; i++ {
		partNum := i + 1

		var partData []byte
		if partNum < totalParts {
			n, err := io.ReadFull(body, buf)
			if err != nil {
				return drivers.Entry{}, fmt.Errorf("139 upload: read part %d: %w", partNum, err)
			}
			partData = buf[:n]
		} else {
			var err error
			partData, err = io.ReadAll(body)
			if err != nil {
				return drivers.Entry{}, fmt.Errorf("139 upload: read last part: %w", err)
			}
		}

		var uploadURL string
		for _, p := range preResp.Data.Parts {
			if p.PartNumber == partNum {
				uploadURL = p.UploadUrl
				break
			}
		}
		if uploadURL == "" {
			return drivers.Entry{}, fmt.Errorf("139 upload: no url for part %d", partNum)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(partData))
		if err != nil {
			return drivers.Entry{}, fmt.Errorf("139 upload part %d: %w", partNum, err)
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Authorization", d.cl.getAuthorization())
		resp, err := d.cl.httpClient.Do(req)
		if err != nil {
			return drivers.Entry{}, fmt.Errorf("139 upload part %d: %w", partNum, err)
		}
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return drivers.Entry{}, fmt.Errorf("139 upload part %d: status %d", partNum, resp.StatusCode)
		}
	}

	commitData := map[string]interface{}{
		"fileId": preResp.Data.FileId,
	}
	var commitResp uploadCommitResp
	err = d.cl.doRequest(http.MethodPost, "/file/upload/complete", commitData, &commitResp)
	if err != nil {
		return drivers.Entry{}, fmt.Errorf("139 upload complete: %w", err)
	}
	if !commitResp.Success {
		return drivers.Entry{}, fmt.Errorf("139 upload complete failed: %s", commitResp.Message)
	}

	return drivers.Entry{ID: commitResp.Data.FileId, Name: name, Size: size}, nil
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
