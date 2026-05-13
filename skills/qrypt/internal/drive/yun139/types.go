package yun139

import (
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/drive"
)

type baseResp struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type personalListResp struct {
	baseResp
	Data struct {
		NextPageCursor string         `json:"nextPageCursor"`
		Items          []personalItem `json:"items"`
	} `json:"data"`
}

type personalItem struct {
	FileId     string        `json:"fileId"`
	Name       string        `json:"name"`
	Type       string        `json:"type"`
	Size       int64         `json:"size"`
	CreatedAt  string        `json:"createdAt"`
	UpdatedAt  string        `json:"updatedAt"`
	Thumbnails []thumbEntry  `json:"thumbnailUrlList"`
}

type thumbEntry struct {
	Style string `json:"style"`
	Url   string `json:"url"`
}

func personalTime(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02T15:04:05.999-07:00", s, time.Local)
	if err != nil {
		return time.Now()
	}
	return t
}

func toEntry(item personalItem) drive.Entry {
	return drive.Entry{
		ID:      item.FileId,
		Name:    item.Name,
		IsDir:   item.Type == "folder",
		Size:    item.Size,
		ModTime: personalTime(item.UpdatedAt),
	}
}

func toEntries(items []personalItem) []drive.Entry {
	entries := make([]drive.Entry, len(items))
	for i := range items {
		entries[i] = toEntry(items[i])
	}
	return entries
}

type createResp struct {
	baseResp
	Data struct {
		FileId string `json:"fileId"`
		Name   string `json:"name"`
		Type   string `json:"type"`
	} `json:"data"`
}

type downloadResp struct {
	baseResp
	Data struct {
		Url    string `json:"url"`
		CdnUrl string `json:"cdnUrl"`
	} `json:"data"`
}

type uploadPreResp struct {
	baseResp
	Data struct {
		FileId     string     `json:"fileId"`
		UploadUrl  string     `json:"uploadUrl"`
		UploadId   string     `json:"uploadId"`
		PartNumber int        `json:"partNumber"`
		PartSize   int64      `json:"partSize"`
		Parts      []partInfo `json:"partInfoList"`
	} `json:"data"`
}

type partInfo struct {
	PartNumber int    `json:"partNumber"`
	UploadUrl  string `json:"uploadUrl"`
}

type uploadCommitResp struct {
	baseResp
	Data struct {
		FileId string `json:"fileId"`
	} `json:"data"`
}
