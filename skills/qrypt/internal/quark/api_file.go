package quark

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
)

type FileService struct {
	client  *Client
	cache   *CacheService
	cipher  crypt.Cipher
}

func NewFileService(client *Client, cache *CacheService, cipher crypt.Cipher) *FileService {
	return &FileService{client: client, cache: cache, cipher: cipher}
}

func (s *FileService) Client() *Client { return s.client }
func (s *FileService) Cache() *CacheService { return s.cache }

func (s *FileService) SetCipher(cipher crypt.Cipher) {
	s.cipher = cipher
}

func (s *FileService) Auth() error {
	if s.client.Cookie() == "" {
		return fmt.Errorf("cookie is required")
	}
	var resp SortResp
	err := s.client.Request(http.MethodGet, "/file/sort", map[string]string{
		"pdir_fid": "0",
		"_size":    "1",
	}, nil, &resp)
	if err != nil {
		return err
	}
	if resp.Status >= 400 || resp.Code != 0 {
		return fmt.Errorf("auth failed: %s", resp.Message)
	}
	return nil
}

func (s *FileService) ListFiles(parentFid string) ([]File, error) {
	if val, ok := s.cache.GetDir(parentFid); ok {
		return val, nil
	}

	size := 100
	var firstResp SortResp
	err := s.client.Request(http.MethodGet, "/file/sort", map[string]string{
		"pdir_fid":             parentFid,
		"_size":                strconv.Itoa(size),
		"_page":                "1",
		"_fetch_total":         "1",
		"fetch_all_file":       "1",
		"fetch_risk_file_name": "1",
	}, nil, &firstResp)
	if err != nil {
		return nil, err
	}
	if firstResp.Status >= 400 || firstResp.Code != 0 {
		return nil, fmt.Errorf("list files failed: %s", firstResp.Message)
	}

	total := firstResp.Metadata.Total
	allFiles := make([]File, total)
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
				var resp SortResp
				err := s.client.Request(http.MethodGet, "/file/sort", map[string]string{
					"pdir_fid":             parentFid,
					"_size":                strconv.Itoa(size),
					"_page":                strconv.Itoa(page),
					"fetch_all_file":       "1",
					"fetch_risk_file_name": "1",
				}, nil, &resp)
				if err != nil {
					errOnce.Do(func() { lastErr = err })
					return
				}
				if resp.Status >= 400 || resp.Code != 0 {
					errOnce.Do(func() { lastErr = fmt.Errorf("%s", resp.Message) })
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

	s.cache.SetDir(parentFid, allFiles)
	return allFiles, nil
}

func (s *FileService) FindChildByName(parentFid, name string) (string, error) {
	if s.cache.GetNeg(parentFid, name) {
		return "", fmt.Errorf("child not found (cached): %s", name)
	}

	files, err := s.ListFiles(parentFid)
	if err != nil {
		return "", err
	}

	for _, f := range files {
		if f.FileName == name {
			s.cache.DeleteNeg(parentFid, name)
			return f.Fid, nil
		}
	}

	s.cache.SetNeg(parentFid, name)
	return "", fmt.Errorf("child not found: %s", name)
}

func (s *FileService) ResolvePath(path string) (string, error) {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	currentFid := "0"
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		files, err := s.ListFiles(currentFid)
		if err != nil {
			return "", err
		}
		found := false
		encSeg := ""
		if s.cipher != nil {
			encSeg = s.cipher.EncryptSegment(seg)
		}
		for _, f := range files {
			if f.FileName == seg {
				currentFid = f.Fid
				found = true
				break
			}
			if encSeg != "" && strings.EqualFold(f.FileName, encSeg) {
				currentFid = f.Fid
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("child not found: %s", seg)
		}
	}
	return currentFid, nil
}

func (s *FileService) GetDownloadURL(fid string) (string, error) {
	if val, ok := s.cache.GetURL(fid); ok {
		return val, nil
	}

	data := map[string]interface{}{
		"fids": []string{fid},
	}
	var resp DownResp
	err := s.client.Request(http.MethodPost, "/file/download", nil, data, &resp)
	if err != nil {
		return "", err
	}
	if resp.Status >= 400 || resp.Code != 0 {
		return "", fmt.Errorf("get download url failed: %s", resp.Message)
	}
	if len(resp.Data) == 0 {
		return "", fmt.Errorf("no download url found")
	}
	url := resp.Data[0].DownloadUrl
	s.cache.SetURL(fid, url)
	return url, nil
}
