package driver

import (
	"encoding/json"
	"fmt"
	"time"
)

type Logger interface {
	Printf(format string, v ...interface{})
	Debug(args ...interface{})
	Debugf(format string, v ...interface{})
	Info(args ...interface{})
	Infof(format string, v ...interface{})
	Warn(args ...interface{})
	Warnf(format string, v ...interface{})
	Error(args ...interface{})
	Errorf(format string, v ...interface{})
}

type StdLogger struct{}

func (l *StdLogger) Printf(format string, v ...interface{}) {
	fmt.Printf(format, v...)
}
func (l *StdLogger) Debug(args ...interface{})                 { fmt.Print(args...) }
func (l *StdLogger) Debugf(format string, v ...interface{})   { fmt.Printf(format, v...) }
func (l *StdLogger) Info(args ...interface{})                  { fmt.Print(args...) }
func (l *StdLogger) Infof(format string, v ...interface{})    { fmt.Printf(format, v...) }
func (l *StdLogger) Warn(args ...interface{})                  { fmt.Print(args...) }
func (l *StdLogger) Warnf(format string, v ...interface{})    { fmt.Printf(format, v...) }
func (l *StdLogger) Error(args ...interface{})                 { fmt.Print(args...) }
func (l *StdLogger) Errorf(format string, v ...interface{})   { fmt.Printf(format, v...) }

var Log Logger = &StdLogger{}

type Resp struct {
	Status  int    `json:"status"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	// Quark Error Codes
	QuarkErrFileNotFound = "23001"
	QuarkErrAlreadyDeleted = "23004"
	QuarkErrDirAlreadyExists = "23008"
)

type File struct {
	Fid        string `json:"fid"`
	FileName   string `json:"file_name"`
	Category   int    `json:"category"`
	Size       json.Number `json:"size"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
	File       bool   `json:"file"`
}

func (f *File) Int64Size() int64 {
	v, _ := f.Size.Int64()
	return v
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

type UploadCallback struct {
	CallbackUrl  string `json:"callbackUrl"`
	CallbackBody string `json:"callbackBody"`
}

type HashResp struct {
	Resp
	Data struct {
		Finish bool `json:"finish"`
		Fid    string `json:"fid"`
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
		Finish    bool   `json:"finish"` // 秒传标记
		Bucket    string `json:"bucket"`
		Callback  json.RawMessage `json:"callback"`
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
