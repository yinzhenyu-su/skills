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

// verifyFileName checks if a file with the given fid has the expected plaintext name
// in the parent directory. Used to verify dedup results before accepting them.
func (m *Manager) verifyFileName(fid, parentFid, expectedPlainName string) bool {
	if parentFid == "" || parentFid == "0" || parentFid == "root" {
		return true // can't verify at root, assume OK
	}
	files, err := m.driver.ListFiles(parentFid)
	if err != nil {
		return false
	}
	for _, f := range files {
		if f.Fid == fid {
			decName, decErr := m.cipher.DecryptSegment(f.FileName)
			if decErr != nil {
				return false
			}
			return decName == expectedPlainName
		}
	}
	return false
}

// deleteExistingFileByName lists files in parentFid and deletes any file with matching
// plaintext name. This prevents duplicate files with (1) suffix when re-uploading.
// Uses plaintext comparison because EncryptSegment generates a different ciphertext
// each time (random nonce), so comparing encrypted names won't find previous uploads.
func (m *Manager) deleteExistingFileByName(parentFid, plainName string) error {
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

	files, err := m.driver.ListFiles(parentFid)
	if err != nil {
		driver.Log.Printf("deleteExistingFileByName: warning: failed to list files in parent %s: %v\n", parentFid, err)
		return nil
	}

	// [DEBUG] 打印所有文件，用于排查 (1) 重名问题
	driver.Log.Printf("deleteExistingFileByName: ListFiles returned %d files in parent %s, looking for plainName=%s\n",
		len(files), parentFid, plainName)
	for _, f := range files {
		decName, decErr := m.cipher.DecryptSegment(f.FileName)
		driver.Log.Printf("deleteExistingFileByName:   fid=%s enc=%s dec=%s size=%d decErr=%v\n",
			f.Fid, f.FileName, decName, f.Int64Size(), decErr)
	}

	for _, f := range files {
		decName, decErr := m.cipher.DecryptSegment(f.FileName)
		if decErr != nil {
			continue
		}
		if decName == plainName {
			driver.Log.Printf("deleteExistingFileByName: found existing file %s (fid=%s), deleting before re-upload\n", decName, f.Fid)
			if err := m.driver.Delete([]string{f.Fid}); err != nil {
				driver.Log.Printf("deleteExistingFileByName: warning: failed to delete existing file: %v\n", err)
				return nil
			}
			// Invalidate cache ONLY after a successful deletion
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

	preStart := time.Now()
	pre, err := m.driver.UploadPre(encName, req.ParentFid, encSize)
	result.PreDuration = time.Since(preStart)
	if err != nil {
		return result, err
	}

	// [DEBUG] UploadPre 结果，用于排查 (1) 重名问题
	driver.Log.Printf("Sync DEBUG: UploadPre result for %s: finish=%v fid=%s encName=%s plainSize=%d encSize=%d\n",
		req.Name, pre.Data.Finish, pre.Data.Fid, encName, req.PlainSize, encSize)

	// If UploadPre returned finish=true (dedup), verify the dedup file has the
	// correct name. If not, it's a hash collision or stale dedup — delete the
	// old file and re-create UploadPre to get a fresh upload task.
	for i := 0; pre.Data.Finish && pre.Data.Fid != "" && i < 3; i++ {
		if m.verifyFileName(pre.Data.Fid, req.ParentFid, req.Name) {
			break // Dedup file has correct name — accept it
		}
		driver.Log.Printf("Sync: dedup fid=%s has wrong name, deleting old file and re-creating upload for %s (attempt %d)\n", pre.Data.Fid, req.Name, i+1)
		// Clean up the placeholder created by UploadPre
		m.driver.UploadFinish(pre)
		// Delete the old file with same plaintext name
		m.deleteExistingFileByName(req.ParentFid, req.Name)
		// Re-create UploadPre for a fresh upload
		pre, err = m.driver.UploadPre(encName, req.ParentFid, encSize)
		if err != nil {
			return result, err
		}
	}

	// If not dedup, delete existing file with same name to prevent (1) duplicates
	if !pre.Data.Finish {
		driver.Log.Printf("Sync DEBUG: not dedup, calling deleteExistingFileByName for %s in parent %s\n", req.Name, req.ParentFid)
		if err := m.deleteExistingFileByName(req.ParentFid, req.Name); err != nil {
			driver.Log.Printf("Sync: warning: failed to check/delete existing file %s in parent %s: %v\n", req.Name, req.ParentFid, err)
		}
	}

	if pre.Data.Finish {
		// Verify dedup file name one more time before accepting
		if !m.verifyFileName(pre.Data.Fid, req.ParentFid, req.Name) {
			return result, fmt.Errorf("dedup returned wrong file after retries: fid=%s for %s", pre.Data.Fid, req.Name)
		}
		// Dedup verified: file with correct hash and name already exists
		driver.Log.Printf("Sync: dedup OK for %s (fid=%s)\n", req.Name, pre.Data.Fid)
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
