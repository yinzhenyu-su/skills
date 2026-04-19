package upload

import (
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/driver"
	"github.com/yinzhenyu/skills/qrypt/internal/staging"
)

type SyncRequest struct {
	Path                 string
	Name                 string
	ParentFid            string
	LocalPath            string
	PlainSize            int64
	SkipDeleteExisting   bool // if true, skip deleteExistingFileByName (for external/test uploads)
}

type SyncResult struct {
	Fid                string
	Nonce              [24]byte
	EncryptedSize      int64
	PartCount          int
	UploadedBytes      int64
	PreDuration        time.Duration
	UpdateHashDuration time.Duration
	UploadPartDuration time.Duration
	CommitDuration     time.Duration
	FinishDuration     time.Duration
}

type Manager struct {
	driver  *driver.QuarkDriver
	cipher  *crypt.RcloneCipher
	staging *staging.Store
}

func NewManager(d *driver.QuarkDriver, c *crypt.RcloneCipher, s *staging.Store) *Manager {
	return &Manager{driver: d, cipher: c, staging: s}
}

// deleteExistingFileByName lists files in parentFid and deletes any file with matching encrypted name.
// This prevents duplicate files with (1) suffix when re-uploading an edited file.
// It also invalidates the directory cache before listing to ensure fresh data.
func (m *Manager) deleteExistingFileByName(parentFid, encName string) error {
	// Skip for root directory or empty parent
	if parentFid == "" || parentFid == "0" || parentFid == "root" {
		return nil
	}

	// Recover from panics (e.g., mock drivers in tests may not implement ListFiles fully)
	defer func() {
		if r := recover(); r != nil {
			driver.Log.Printf("deleteExistingFileByName: recovered from panic: %v\n", r)
		}
	}()

	// Use driver's cache for listing. It has a TTL (default 60s).
	// This avoids O(N^2) API calls during batch uploads.
	files, err := m.driver.ListFiles(parentFid)
	if err != nil {
		driver.Log.Printf("deleteExistingFileByName: warning: failed to list files in parent %s: %v\n", parentFid, err)
		return nil
	}

	for _, f := range files {
		if f.FileName == encName {
			driver.Log.Printf("deleteExistingFileByName: found existing file %s (fid=%s), deleting before re-upload\n", encName, f.Fid)
			if err := m.driver.Delete([]string{f.Fid}); err != nil {
				driver.Log.Printf("deleteExistingFileByName: warning: failed to delete existing file: %v\n", err)
				return nil
			}
			// Invalidate cache ONLY after a successful deletion so subsequent ListFiles or UploadPre sees the change.
			m.driver.RemoveDirCache(parentFid)
			return nil
		}
	}
	return nil
}

func (m *Manager) Sync(req SyncRequest) (SyncResult, error) {
	var result SyncResult
	if req.LocalPath == "" {
		return result, fmt.Errorf("missing staging file for %s", req.Path)
	}

	nonce, err := m.cipher.GenerateRandomNonce()
	if err != nil {
		return result, err
	}

	encName := m.cipher.EncryptSegment(req.Name)
	encSize := m.cipher.EncryptedSize(req.PlainSize)
	result.Nonce = nonce
	result.EncryptedSize = encSize

	// Check if file with same name already exists in parent directory.
	// If so, delete it first to avoid duplicate files with (1) suffix.
	// Skip for external/test uploads that intentionally add a file alongside existing ones.
	if !req.SkipDeleteExisting {
		if err := m.deleteExistingFileByName(req.ParentFid, encName); err != nil {
			driver.Log.Printf("Sync: warning: failed to check/delete existing file %s in parent %s: %v\n", encName, req.ParentFid, err)
		}
	}

	preStart := time.Now()
	pre, err := m.driver.UploadPre(encName, req.ParentFid, encSize)
	result.PreDuration = time.Since(preStart)
	if err != nil {
		return result, err
	}

	if pre.Data.Finish {
		// 秒传/去重：文件已存在，但仍需 UploadFinish 清理 UploadPre 创建的占位文件
		if err := m.driver.UploadFinish(pre); err != nil {
			driver.Log.Printf("Sync: UploadFinish after dedup failed for %s: %v\n", req.Path, err)
		}
		result.Fid = pre.Data.Fid
		return result, nil
	}

	partSize := pre.Metadata.PartSize
	if partSize <= 0 {
		return result, fmt.Errorf("invalid multipart part size: %d", partSize)
	}

	reader, err := m.staging.OpenReader(req.LocalPath)
	if err != nil {
		return result, err
	}
	defer reader.Close()

	encReader := crypt.NewEncryptingReader(reader, m.cipher, nonce, req.PlainSize)
	buf := make([]byte, partSize)
	etags := make([]string, 0, max(1, int((encSize+int64(partSize)-1)/int64(partSize))))
	md5h := md5.New()
	sha1h := sha1.New()

	for partNumber := 1; ; partNumber++ {
		n, readErr := io.ReadFull(encReader, buf)
		if readErr == io.EOF && n == 0 {
			break
		}
		if readErr != nil && readErr != io.ErrUnexpectedEOF {
			return result, readErr
		}
		if _, err := md5h.Write(buf[:n]); err != nil {
			return result, err
		}
		if _, err := sha1h.Write(buf[:n]); err != nil {
			return result, err
		}

		partStart := time.Now()
		etag, err := m.driver.UploadPart(pre, partNumber, buf[:n])
		result.UploadPartDuration += time.Since(partStart)
		if err != nil {
			return result, err
		}
		etags = append(etags, etag)
		result.PartCount++
		result.UploadedBytes += int64(n)

		if readErr == io.ErrUnexpectedEOF {
			break
		}
	}

	hashStart := time.Now()
	finish, fid, err := m.driver.UpdateHash(strings.ToUpper(hex.EncodeToString(md5h.Sum(nil))), strings.ToUpper(hex.EncodeToString(sha1h.Sum(nil))), pre.Data.TaskId)
	result.UpdateHashDuration = time.Since(hashStart)
	if err != nil {
		return result, err
	}
	if finish {
		if fid != "" {
			result.Fid = fid
		} else {
			result.Fid = pre.Data.Fid
		}
		return result, nil
	}

	commitStart := time.Now()
	if err := m.driver.UploadCommit(pre, etags); err != nil {
		result.CommitDuration = time.Since(commitStart)
		return result, err
	}
	result.CommitDuration = time.Since(commitStart)

	finishStart := time.Now()
	if err := m.driver.UploadFinish(pre); err != nil {
		result.FinishDuration = time.Since(finishStart)
		return result, err
	}
	result.FinishDuration = time.Since(finishStart)
	result.Fid = pre.Data.Fid
	return result, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
