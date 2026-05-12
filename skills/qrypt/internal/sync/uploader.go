package sync

import (
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/log"
	"github.com/yinzhenyu/skills/qrypt/internal/quark"
)

type Request struct {
	Path       string
	Name       string
	ParentFid  string
	PlainSize  int64
	OldFid     string
	Nonce      [24]byte
	UploadID   string
	LastPart   int
	DataReader func() (io.ReadCloser, error)
	ProgressFn func(partNumber int)
}

type Result struct {
	Fid           string
	Nonce         [24]byte
	EncryptedSize int64
	PartCount     int
	UploadedBytes int64
	UploadID      string
}

type Uploader struct {
	fileSvc    *quark.FileService
	manageSvc  *quark.ManageService
	uploadSvc  *quark.UploadService
	cacheSvc   *quark.CacheService
	cipher     *crypt.RcloneCipher
	maxRetries int
}

func NewUploader(
	fileSvc *quark.FileService,
	manageSvc *quark.ManageService,
	uploadSvc *quark.UploadService,
	cacheSvc *quark.CacheService,
	cipher *crypt.RcloneCipher,
) *Uploader {
	return &Uploader{
		fileSvc:    fileSvc,
		manageSvc:  manageSvc,
		uploadSvc:  uploadSvc,
		cacheSvc:   cacheSvc,
		cipher:     cipher,
		maxRetries: 3,
	}
}

const partRetryMax = 3

func partRetryBackoff(attempt int) time.Duration {
	base := time.Duration(200<<uint(attempt)) * time.Millisecond
	jitter := float64(75+(attempt*11)%50) / 100.0
	return time.Duration(float64(base) * jitter)
}

func (u *Uploader) Upload(req Request) (Result, error) {
	var result Result
	if req.DataReader == nil {
		return result, fmt.Errorf("missing data reader for %s", req.Path)
	}

	nonce := req.Nonce
	isResume := req.UploadID != "" || !isZeroNonce(nonce)
	if isResume {
		log.L.Infof("Upload: RESUMING for %s (uploadID=%s, nonce=%v)\n", req.Path, req.UploadID, !isZeroNonce(nonce))
	}
	if isZeroNonce(nonce) {
		var err error
		nonce, err = u.cipher.GenerateRandomNonce()
		if err != nil {
			return result, err
		}
	}

	encName := u.cipher.EncryptSegment(req.Name)
	encSize := u.cipher.EncryptedSize(req.PlainSize)
	result.Nonce = nonce
	result.EncryptedSize = encSize

	log.L.Debugf("Sync: deleteExistingFileByName for %s in parent %s\n", req.Name, req.ParentFid)
	u.deleteExistingFileByName(req.ParentFid, req.Name)

	log.L.Debugf("Sync: UploadPre for %s encName=%s plainSize=%d encSize=%d\n", req.Name, encName, req.PlainSize, encSize)
	pre, err := u.uploadSvc.UploadPre(encName, req.ParentFid, encSize, req.UploadID)
	if err != nil {
		return result, err
	}
	result.UploadID = pre.Data.UploadId

	for i := 0; pre.Data.Finish && pre.Data.Fid != "" && i < 3; i++ {
		log.L.Infof("Sync: dedup fid=%s has wrong name, re-creating upload for %s (attempt %d)\n", pre.Data.Fid, req.Name, i+1)
		if u.verifyFileName(pre.Data.Fid, req.ParentFid, req.Name) {
			break
		}
		u.uploadSvc.UploadFinish(pre)
		u.deleteExistingFileByName(req.ParentFid, req.Name)
		pre, err = u.uploadSvc.UploadPre(encName, req.ParentFid, encSize, "")
		if err != nil {
			return result, err
		}
	}

	if pre.Data.Finish {
		if !u.verifyFileName(pre.Data.Fid, req.ParentFid, req.Name) {
			return result, fmt.Errorf("dedup returned wrong file after retries: fid=%s for %s", pre.Data.Fid, req.Name)
		}
		log.L.Infof("Sync: dedup OK for %s (fid=%s)\n", req.Name, pre.Data.Fid)
		u.uploadSvc.UploadFinish(pre)
		result.Fid = pre.Data.Fid
		return result, nil
	}

	partSize := pre.Metadata.PartSize
	if partSize <= 0 {
		return result, fmt.Errorf("invalid multipart part size: %d", partSize)
	}

	reader, err := req.DataReader()
	if err != nil {
		return result, err
	}
	defer reader.Close()

	encReader := crypt.NewEncryptingReader(reader, u.cipher, nonce, req.PlainSize)
	buf := make([]byte, partSize)
	etags := make([]string, 0, max(1, int((encSize+int64(partSize)-1)/int64(partSize))))
	md5h := md5.New()
	sha1h := sha1.New()
	hashTee := io.TeeReader(encReader, io.MultiWriter(md5h, sha1h))

	skipBytes := int64(req.LastPart) * int64(partSize)
	if skipBytes > 0 {
		if _, err := io.CopyN(io.Discard, hashTee, skipBytes); err != nil && err != io.EOF {
			return result, fmt.Errorf("skip encrypted failed: %w", err)
		}
	}

	for partNumber := req.LastPart + 1; ; partNumber++ {
		n, readErr := io.ReadFull(hashTee, buf)
		if readErr == io.EOF && n == 0 {
			break
		}
		if readErr != nil && readErr != io.ErrUnexpectedEOF {
			return result, readErr
		}

		var etag string
		for attempt := 0; attempt < partRetryMax; attempt++ {
			etag, err = u.uploadSvc.UploadPart(pre, partNumber, buf[:n])
			if err == nil {
				break
			}
			if attempt < partRetryMax-1 {
				time.Sleep(partRetryBackoff(attempt))
			}
		}
		if err != nil {
			return result, fmt.Errorf("UploadPart %d failed after %d retries: %v", partNumber, partRetryMax, err)
		}
		etags = append(etags, etag)
		result.PartCount++
		result.UploadedBytes += int64(n)

		if req.ProgressFn != nil {
			req.ProgressFn(partNumber)
		}
		if readErr == io.ErrUnexpectedEOF {
			break
		}
	}

	finish, fid, err := u.uploadSvc.UpdateHash(
		strings.ToUpper(hex.EncodeToString(md5h.Sum(nil))),
		strings.ToUpper(hex.EncodeToString(sha1h.Sum(nil))),
		pre.Data.TaskId)
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

	if err := u.uploadSvc.UploadCommit(pre, etags); err != nil {
		return result, err
	}
	if err := u.uploadSvc.UploadFinish(pre); err != nil {
		return result, err
	}
	result.Fid = pre.Data.Fid
	return result, nil
}

func (u *Uploader) verifyFileName(fid, parentFid, expectedPlainName string) bool {
	if parentFid == "" || parentFid == "0" || parentFid == "root" {
		return true
	}
	files, err := u.fileSvc.ListFiles(parentFid)
	if err != nil {
		return false
	}
	for _, f := range files {
		if f.Fid == fid {
			decName, decErr := u.cipher.DecryptSegment(f.FileName)
			if decErr != nil {
				return false
			}
			return decName == expectedPlainName
		}
	}
	return false
}

func (u *Uploader) deleteExistingFileByName(parentFid, plainName string) error {
	if parentFid == "" || parentFid == "0" || parentFid == "root" {
		return nil
	}
	files, err := u.fileSvc.ListFiles(parentFid)
	if err != nil {
		return nil
	}
	var deleteFids []string
	for _, f := range files {
		decName, decErr := u.cipher.DecryptSegment(f.FileName)
		if decErr != nil {
			if crypt.HasConflictSuffix(f.FileName) {
				continue
			}
		}
		if decName == plainName {
			deleteFids = append(deleteFids, f.Fid)
		}
	}
	if len(deleteFids) > 0 {
		u.manageSvc.Delete(deleteFids)
		u.cacheSvc.RemoveDir(parentFid)
	}
	return nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func isZeroNonce(n [24]byte) bool {
	for _, b := range n {
		if b != 0 {
			return false
		}
	}
	return true
}
