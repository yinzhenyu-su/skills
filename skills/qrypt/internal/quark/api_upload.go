package quark

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type UploadService struct {
	client *Client
}

func NewUploadService(client *Client) *UploadService {
	return &UploadService{client: client}
}

func (s *UploadService) UploadPre(fileName, parentFid string, size int64, uploadID string) (*UpPreResp, error) {
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
	if uploadID != "" {
		data["upload_id"] = uploadID
	}
	var resp UpPreResp
	err := s.client.Request(http.MethodPost, "/file/upload/pre", nil, data, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (s *UploadService) getOSSURL(pre *UpPreResp) string {
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

func (s *UploadService) UploadPart(pre *UpPreResp, partNumber int, data []byte) (string, error) {
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
		var authResp UpAuthResp
		err := s.client.Request(http.MethodPost, "/file/upload/auth", nil, authData, &authResp)
		if err != nil {
			if attempt < ossMaxRetries {
				time.Sleep(retryBackoff(attempt))
				continue
			}
			return "", err
		}

		u := s.getOSSURL(pre)
		q := "?partNumber=" + strconv.Itoa(partNumber) + "&uploadId=" + pre.Data.UploadId
		req, err := http.NewRequest(http.MethodPut, u+q, bytes.NewReader(data))
		if err != nil {
			return "", err
		}

		req.Header.Set("Authorization", authResp.Data.AuthKey)
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("x-oss-date", dateStr)
		req.Header.Set("x-oss-user-agent", "aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit")
		req.Header.Set("Referer", Referer)
		req.Header.Set("User-Agent", UserAgent)

		resp, err := s.client.httpClient.Do(req)
		if err != nil {
			if attempt < ossMaxRetries && isRetryableHTTPError(err) {
				time.Sleep(retryBackoff(attempt))
				continue
			}
			return "", err
		}

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if attempt < ossMaxRetries && isRetryableHTTPStatus(resp.StatusCode) {
				time.Sleep(retryBackoff(attempt))
				continue
			}
			return "", fmt.Errorf("oss put status: %d, error: %s", resp.StatusCode, string(bodyBytes))
		}

		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		return resp.Header.Get("Etag"), nil
	}
	return "", fmt.Errorf("oss upload part %d failed after %d retries", partNumber, ossMaxRetries+1)
}

func (s *UploadService) UpdateHash(md5Hex, sha1Hex, taskID string) (bool, string, error) {
	data := map[string]interface{}{
		"md5":     md5Hex,
		"sha1":    sha1Hex,
		"task_id": taskID,
	}
	var resp HashResp
	err := s.client.Request(http.MethodPost, "/file/update/hash", nil, data, &resp)
	if err != nil {
		return false, "", err
	}
	return resp.Data.Finish, resp.Data.Fid, nil
}

func encodeOSSCallback(callbackRaw []byte) (string, error) {
	if len(callbackRaw) == 0 {
		return "", fmt.Errorf("missing callback payload")
	}
	return base64.StdEncoding.EncodeToString(callbackRaw), nil
}

func (s *UploadService) UploadCommit(pre *UpPreResp, etags []string) error {
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

	m := md5.New()
	m.Write([]byte(body))
	contentMd5 := base64.StdEncoding.EncodeToString(m.Sum(nil))

	for attempt := 0; attempt <= ossMaxRetries; attempt++ {
		timeStr := time.Now().UTC().Format(http.TimeFormat)
		callbackBase64, err := encodeOSSCallback([]byte(pre.Data.Callback))
		if err != nil {
			return err
		}

		authMeta := fmt.Sprintf("POST\n%s\napplication/xml\n%s\nx-oss-callback:%s\nx-oss-date:%s\nx-oss-user-agent:aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit\n/%s/%s?uploadId=%s",
			contentMd5, timeStr, callbackBase64, timeStr, pre.Data.Bucket, pre.Data.ObjKey, pre.Data.UploadId)

		authData := map[string]interface{}{
			"auth_info": pre.Data.AuthInfo,
			"auth_meta": authMeta,
			"task_id":   pre.Data.TaskId,
		}
		var authResp UpAuthResp
		err = s.client.Request(http.MethodPost, "/file/upload/auth", nil, authData, &authResp)
		if err != nil {
			if attempt < ossMaxRetries {
				time.Sleep(retryBackoff(attempt))
				continue
			}
			return err
		}

		u := s.getOSSURL(pre)
		q := "?uploadId=" + pre.Data.UploadId
		req, err := http.NewRequest(http.MethodPost, u+q, strings.NewReader(body))
		if err != nil {
			return err
		}

		req.Header.Set("Authorization", authResp.Data.AuthKey)
		req.Header.Set("Content-MD5", contentMd5)
		req.Header.Set("Content-Type", "application/xml")
		req.Header.Set("x-oss-callback", callbackBase64)
		req.Header.Set("x-oss-date", timeStr)
		req.Header.Set("x-oss-user-agent", "aliyun-sdk-js/6.6.1 Chrome 98.0.4758.80 on Windows 10 64-bit")
		req.Header.Set("Referer", Referer)
		req.Header.Set("User-Agent", UserAgent)

		resp, err := s.client.httpClient.Do(req)
		if err != nil {
			if attempt < ossMaxRetries && isRetryableHTTPError(err) {
				time.Sleep(retryBackoff(attempt))
				continue
			}
			return err
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			if attempt < ossMaxRetries && isRetryableHTTPStatus(resp.StatusCode) {
				time.Sleep(retryBackoff(attempt))
				continue
			}
			return fmt.Errorf("oss commit status: %d, error: %s", resp.StatusCode, string(bodyBytes))
		}
		return nil
	}
	return fmt.Errorf("oss commit failed after %d retries", ossMaxRetries+1)
}

func (s *UploadService) UploadFinish(pre *UpPreResp) error {
	data := map[string]interface{}{
		"obj_key": pre.Data.ObjKey,
		"task_id": pre.Data.TaskId,
	}
	var resp Resp
	err := s.client.Request(http.MethodPost, "/file/upload/finish", nil, data, &resp)
	if err != nil {
		return err
	}
	if resp.Status >= 400 || resp.Code != 0 {
		return fmt.Errorf("UploadFinish error: status=%d, code=%d, message=%s", resp.Status, resp.Code, resp.Message)
	}
	return nil
}
