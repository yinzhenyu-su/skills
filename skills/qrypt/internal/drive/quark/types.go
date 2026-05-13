package quark

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/drive"
)

type file struct {
	Fid       string      `json:"fid"`
	FileName  string      `json:"file_name"`
	Category  int         `json:"category"`
	Size      json.Number `json:"size"`
	CreatedAt int64       `json:"created_at"`
	UpdatedAt int64       `json:"updated_at"`
	File      bool        `json:"file"`
}

func (f *file) isDir() bool   { return !f.File }
func (f *file) int64Size() int64 {
	v, _ := f.Size.Int64()
	return v
}
func (f *file) modTime() time.Time {
	if f.UpdatedAt > 0 {
		return time.UnixMilli(f.UpdatedAt)
	}
	return time.UnixMilli(f.CreatedAt)
}

type quarkFileMeta struct {
	Category int
}

func toEntry(f *file) drive.Entry {
	return drive.Entry{
		ID:      f.Fid,
		Name:    f.FileName,
		IsDir:   f.isDir(),
		Size:    f.int64Size(),
		ModTime: f.modTime(),
		Extra:   quarkFileMeta{Category: f.Category},
	}
}

func toEntries(files []file) []drive.Entry {
	entries := make([]drive.Entry, len(files))
	for i := range files {
		entries[i] = toEntry(&files[i])
	}
	return entries
}

type resp struct {
	Status  int    `json:"status"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type sortResp struct {
	resp
	Data struct {
		List []file `json:"list"`
	} `json:"data"`
	Metadata struct {
		Total int `json:"_total"`
	} `json:"metadata"`
}

type downResp struct {
	resp
	Data []struct {
		DownloadUrl string `json:"download_url"`
	} `json:"data"`
}

type upPreResp struct {
	resp
	Data struct {
		TaskId    string          `json:"task_id"`
		UploadId  string          `json:"upload_id"`
		ObjKey    string          `json:"obj_key"`
		UploadUrl string          `json:"upload_url"`
		Fid       string          `json:"fid"`
		Finish    bool            `json:"finish"`
		Bucket    string          `json:"bucket"`
		Callback  json.RawMessage `json:"callback"`
		AuthInfo  string          `json:"auth_info"`
	} `json:"data"`
	Metadata struct {
		PartSize int `json:"part_size"`
	} `json:"metadata"`
}

type upAuthResp struct {
	resp
	Data struct {
		AuthKey string `json:"auth_key"`
	} `json:"data"`
}

type hashResp struct {
	resp
	Data struct {
		Finish bool   `json:"finish"`
		Fid    string `json:"fid"`
	} `json:"data"`
}

type createDirResp struct {
	resp
	Data struct {
		Fid string `json:"fid"`
	} `json:"data"`
}

type cacheManager struct {
	dirCacheTTL time.Duration
	negCacheTTL time.Duration

	urlCache sync.Map
	dirCache sync.Map
	negCache sync.Map
}

func newCacheManager() *cacheManager {
	return &cacheManager{
		dirCacheTTL: 60 * time.Second,
		negCacheTTL: 60 * time.Second,
	}
}

type cachedURL struct {
	url    string
	expiry time.Time
}

type dirCacheEntry struct {
	files  []file
	expiry time.Time
}

func (m *cacheManager) getURL(fid string) (string, bool) {
	if val, ok := m.urlCache.Load(fid); ok {
		cu := val.(cachedURL)
		if time.Now().Before(cu.expiry) {
			return cu.url, true
		}
	}
	return "", false
}

func (m *cacheManager) setURL(fid, url string) {
	m.urlCache.Store(fid, cachedURL{url: url, expiry: time.Now().Add(10 * time.Minute)})
}

func (m *cacheManager) invalidateURL(fid string) {
	m.urlCache.Delete(fid)
}

func (m *cacheManager) getDir(parentFid string) ([]file, bool) {
	if val, ok := m.dirCache.Load(parentFid); ok {
		entry := val.(dirCacheEntry)
		if time.Now().Before(entry.expiry) {
			return entry.files, true
		}
	}
	return nil, false
}

func (m *cacheManager) setDir(parentFid string, files []file) {
	m.dirCache.Store(parentFid, dirCacheEntry{files: files, expiry: time.Now().Add(m.dirCacheTTL)})
}

func (m *cacheManager) removeDir(parentFid string) {
	m.dirCache.Delete(parentFid)
}

func (m *cacheManager) getNeg(parentFid, name string) bool {
	key := parentFid + ":" + name
	if val, ok := m.negCache.Load(key); ok {
		expiry := val.(time.Time)
		if time.Now().Before(expiry) {
			return true
		}
	}
	return false
}

func (m *cacheManager) setNeg(parentFid, name string) {
	key := parentFid + ":" + name
	m.negCache.Store(key, time.Now().Add(m.negCacheTTL))
}

func (m *cacheManager) deleteNeg(parentFid, name string) {
	key := parentFid + ":" + name
	m.negCache.Delete(key)
}
