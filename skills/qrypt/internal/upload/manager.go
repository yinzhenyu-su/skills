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
	Path      string
	Name      string
	ParentFid string
	LocalPath string
	PlainSize int64
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

	preStart := time.Now()
	pre, err := m.driver.UploadPre(encName, req.ParentFid, encSize)
	result.PreDuration = time.Since(preStart)
	if err != nil {
		return result, err
	}

	if pre.Data.Finish {
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
