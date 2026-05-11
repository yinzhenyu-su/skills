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
	OldFid    string // 上次成功上传的 FID，用于 FID 直接替换（绕过 ListFiles 索引延迟）

	// 断点续传字段
	// Nonce: 非零值时使用此 nonce（复用崩溃前的加密数据），零值时生成新 nonce
	// UploadID: UploadPre 时传入此 ID（复用崩溃前的上传 session），空字符串时创建新 session
	Nonce    [24]byte
	UploadID string
	LastPart int // 已成功上传的最后一个 part 编号（0=首次上传）

	// ProgressFn 可选的上传进度回调，每完成一个分片后调用
	// partNumber 是已成功上传的分片编号。调用方可在此持久化断点续传进度。
	ProgressFn func(partNumber int)
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
	UploadID           string // 本次上传的 upload_id，用于断点续传
}

type Manager struct {
	driver  *driver.QuarkDriver
	cipher  *crypt.RcloneCipher
	staging *staging.Store
}

func NewManager(d *driver.QuarkDriver, c *crypt.RcloneCipher, s *staging.Store) *Manager {
	return &Manager{driver: d, cipher: c, staging: s}
}

const (
	// partRetryMax is the number of per-part upload retry attempts.
	// OSS PUT with the same upload_id + part_number + data is idempotent,
	// so retrying a failed part is safe and efficient.
	partRetryMax = 3
)

// partRetryBackoff returns backoff duration for per-part retry attempts.
// Base: 200ms, 400ms, 800ms with deterministic jitter.
func partRetryBackoff(attempt int) time.Duration {
	base := time.Duration(200<<uint(attempt)) * time.Millisecond
	jitter := float64(75+(attempt*11)%50) / 100.0
	return time.Duration(float64(base) * jitter)
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
			driver.Log.Infof("deleteExistingFileByName: recovered from panic: %v\n", r)
		}
	}()

	files, err := m.driver.ListFiles(parentFid)
	if err != nil {
		driver.Log.Warnf("deleteExistingFileByName: warning: failed to list files in parent %s: %v\n", parentFid, err)
		return nil
	}

	// [DEBUG] 打印所有文件，用于排查 (1) 重名问题
	driver.Log.Infof("deleteExistingFileByName: ListFiles returned %d files in parent %s, looking for plainName=%s\n", len(files), parentFid, plainName)
	for _, f := range files {
		decName, decErr := m.cipher.DecryptSegment(f.FileName)
		driver.Log.Infof("deleteExistingFileByName:   fid=%s enc=%s dec=%s size=%d decErr=%v\n", f.Fid, f.FileName, decName, f.Int64Size(), decErr)
	}

	var deletedCount int
	for _, f := range files {
		decName, decErr := m.cipher.DecryptSegment(f.FileName)
		if decErr != nil {
			// 解密失败（不是有效加密文件名）→ 跳过
			if crypt.HasConflictSuffix(f.FileName) {
				driver.Log.Warnf("deleteExistingFileByName: skipping unencrypted file %s (fid=%s) — not an encrypted name\n", f.FileName, f.Fid)
			}
			continue
		}
		if decName == plainName {
			if crypt.HasConflictSuffix(f.FileName) {
				driver.Log.Warnf("deleteExistingFileByName: cleaning up conflict file %s (fid=%s, dec=%s)\n", f.FileName, f.Fid, decName)
			}
			driver.Log.Infof("deleteExistingFileByName: found existing file %s (fid=%s), deleting before re-upload\n", decName, f.Fid)
			if err := m.driver.Delete([]string{f.Fid}); err != nil {
				driver.Log.Warnf("deleteExistingFileByName: warning: failed to delete existing file: %v\n", err)
				continue
			}
			deletedCount++
		}
	}
	if deletedCount > 0 {
		m.driver.RemoveDirCache(parentFid)
	}
	if deletedCount > 1 {
		driver.Log.Warnf("deleteExistingFileByName: deleted %d files with plainName=%s (included conflict copies)\n", deletedCount, plainName)
	}
	return nil
}

func (m *Manager) Sync(req SyncRequest) (SyncResult, error) {
	var result SyncResult
	var err error
	if req.LocalPath == "" {
		return result, fmt.Errorf("missing staging file for %s", req.Path)
	}

	nonce := req.Nonce
	isResume := req.UploadID != "" || !isZeroNonce(nonce)
	if isResume {
		driver.Log.Infof("Sync: RESUMING upload for %s (uploadID=%s, nonce present=%v)\n",
			req.Path, req.UploadID, !isZeroNonce(nonce))
	}
	if isZeroNonce(nonce) {
		nonce, err = m.cipher.GenerateRandomNonce()
		if err != nil {
			return result, err
		}
	}

	encName := m.cipher.EncryptSegment(req.Name)
	encSize := m.cipher.EncryptedSize(req.PlainSize)
	result.Nonce = nonce
	result.EncryptedSize = encSize

	// 先删除旧文件，再创建 placeholder。
	// 如果有 OldFid（上次上传的 FID），直接用 FID 删除，绕过 ListFiles 索引延迟。
	// 如果没有 OldFid（首次上传），退化为按名删除。
	if req.OldFid != "" {
		driver.Log.Debugf("Sync deleting old file by FID %s (replacing %s in parent %s)\n", req.OldFid, req.Name, req.ParentFid)
		if err := m.driver.Delete([]string{req.OldFid}); err != nil {
			driver.Log.Warnf("Sync: warning: FID delete failed for %s, falling back to name-based delete: %v\n", req.OldFid, err)
			if err := m.deleteExistingFileByName(req.ParentFid, req.Name); err != nil {
				driver.Log.Warnf("Sync: warning: name-based delete also failed for %s in parent %s: %v\n", req.Name, req.ParentFid, err)
			}
		} else {
			m.driver.RemoveDirCache(req.ParentFid)
		}
	} else {
		driver.Log.Debugf("Sync no OldFid, falling back to deleteExistingFileByName for %s in parent %s\n", req.Name, req.ParentFid)
		if err := m.deleteExistingFileByName(req.ParentFid, req.Name); err != nil {
			driver.Log.Warnf("Sync: warning: pre-delete failed for %s in parent %s: %v\n", req.Name, req.ParentFid, err)
		}
	}

	preStart := time.Now()
	pre, err := m.driver.UploadPre(encName, req.ParentFid, encSize, req.UploadID)
	result.PreDuration = time.Since(preStart)
	if err != nil {
		return result, err
	}
	result.UploadID = pre.Data.UploadId

	// [DEBUG] UploadPre 结果，用于排查 (1) 重名问题
	driver.Log.Debugf("Sync UploadPre result for %s: finish=%v fid=%s encName=%s plainSize=%d encSize=%d\n", req.Name, pre.Data.Finish, pre.Data.Fid, encName, req.PlainSize, encSize)

	// If UploadPre returned finish=true (dedup), verify the dedup file has the
	// correct name. If not, it's a hash collision or stale dedup — delete the
	// old file and re-create UploadPre to get a fresh upload task.
	for i := 0; pre.Data.Finish && pre.Data.Fid != "" && i < 3; i++ {
		if m.verifyFileName(pre.Data.Fid, req.ParentFid, req.Name) {
			break // Dedup file has correct name — accept it
		}
		driver.Log.Infof("Sync: dedup fid=%s has wrong name, deleting old file and re-creating upload for %s (attempt %d)\n", pre.Data.Fid, req.Name, i+1)
		// Clean up the placeholder created by UploadPre
		m.driver.UploadFinish(pre)
		// Delete the old file with same plaintext name
		m.deleteExistingFileByName(req.ParentFid, req.Name)
		// Re-create UploadPre for a fresh upload
		pre, err = m.driver.UploadPre(encName, req.ParentFid, encSize, "")
		if err != nil {
			return result, err
		}
	}

	if pre.Data.Finish {
		// Verify dedup file name one more time before accepting
		if !m.verifyFileName(pre.Data.Fid, req.ParentFid, req.Name) {
			return result, fmt.Errorf("dedup returned wrong file after retries: fid=%s for %s", pre.Data.Fid, req.Name)
		}
		// Dedup verified: file with correct hash and name already exists
		driver.Log.Infof("Sync: dedup OK for %s (fid=%s)\n", req.Name, pre.Data.Fid)
		if err := m.driver.UploadFinish(pre); err != nil {
			driver.Log.Errorf("Sync: UploadFinish after dedup failed for %s: %v\n", req.Path, err)
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

	// 断点续传：跳过已上传的加密分片数据
	skipBytes := int64(req.LastPart) * int64(partSize)
	if skipBytes > 0 {
		driver.Log.Infof("Sync: skipping %d already-uploaded encrypted bytes for %s (lastPart=%d)\n",
			skipBytes, req.Path, req.LastPart)
		if err := encReader.SkipEncrypted(skipBytes); err != nil {
			return result, fmt.Errorf("skip encrypted failed: %w", err)
		}
	}

	for partNumber := req.LastPart + 1; ; partNumber++ {
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
		// Per-part retry: OSS PUT with same upload_id + part_number + data is idempotent.
		// The underlying UploadPart also does OSS-level retry, but this adds another layer
		// at the Sync level for resilience against transient auth or connection issues.
		var etag string
		for attempt := 0; attempt < partRetryMax; attempt++ {
			etag, err = m.driver.UploadPart(pre, partNumber, buf[:n])
			if err == nil {
				break
			}
			if attempt < partRetryMax-1 {
				driver.Log.Warnf("Sync: UploadPart %d retry %d/%d for %s: %v\n", partNumber, attempt+1, partRetryMax, req.Path, err)
				time.Sleep(partRetryBackoff(attempt))
			}
		}
		result.UploadPartDuration += time.Since(partStart)
		if err != nil {
			return result, fmt.Errorf("UploadPart %d failed after %d retries: %v", partNumber, partRetryMax, err)
		}
		etags = append(etags, etag)
		result.PartCount++
		result.UploadedBytes += int64(n)

			// 断点续传进度回调
			if req.ProgressFn != nil {
				req.ProgressFn(partNumber)
			}

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

// isZeroNonce checks if a [24]byte nonce is the zero value (indicates "generate new one").
func isZeroNonce(n [24]byte) bool {
	for _, b := range n {
		if b != 0 {
			return false
		}
	}
	return true
}
