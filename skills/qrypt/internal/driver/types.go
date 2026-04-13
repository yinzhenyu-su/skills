package driver

import (
	"time"
)

type Resp struct {
	Status  int    `json:"status"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type File struct {
	Fid        string `json:"fid"`
	FileName   string `json:"file_name"`
	Category   int    `json:"category"`
	Size       int64  `json:"size"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
	File       bool   `json:"file"`
}

func (f *File) IsDir() bool {
	return !f.File
}

func (f *File) ModTime() time.Time {
	if f.UpdatedAt > 0 {
		return time.UnixMilli(f.UpdatedAt)
	}
	return time.UnixMilli(f.CreatedAt)
}

type SortResp struct {
	Resp
	Data struct {
		List []File `json:"list"`
	} `json:"data"`
	Metadata struct {
		Total int `json:"_total"`
	} `json:"metadata"`
}

type DownResp struct {
	Resp
	Data []struct {
		DownloadUrl string `json:"download_url"`
	} `json:"data"`
}

type UpPreResp struct {
	Resp
	Data struct {
		TaskId    string `json:"task_id"`
		UploadId  string `json:"upload_id"`
		ObjKey    string `json:"obj_key"`
		UploadUrl string `json:"upload_url"`
		Fid       string `json:"fid"`
		Bucket    string `json:"bucket"`
		Callback  struct {
			CallbackUrl  string `json:"callbackUrl"`
			CallbackBody string `json:"callbackBody"`
		} `json:"callback"`
		AuthInfo string `json:"auth_info"`
	} `json:"data"`
	Metadata struct {
		PartSize int `json:"part_size"`
	} `json:"metadata"`
}

type UpAuthResp struct {
	Resp
	Data struct {
		AuthKey string `json:"auth_key"`
	} `json:"data"`
}
