package sync

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
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
	drv    drive.Driver
	cipher *crypt.RcloneCipher
}

func NewUploader(drv drive.Driver, cipher *crypt.RcloneCipher) *Uploader {
	return &Uploader{drv: drv, cipher: cipher}
}

func (u *Uploader) Upload(req Request) (Result, error) {
	var result Result
	if req.DataReader == nil {
		return result, fmt.Errorf("missing data reader for %s", req.Path)
	}

	nonce := req.Nonce
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

	reader, err := req.DataReader()
	if err != nil {
		return result, err
	}
	defer reader.Close()

	encReader := crypt.NewEncryptingReader(reader, u.cipher, nonce, req.PlainSize)

	up, ok := u.drv.(drive.Uploader)
	if !ok {
		return result, fmt.Errorf("driver does not support upload")
	}

	entry, err := up.Put(context.Background(), req.ParentFid, encName, encSize, encReader)
	if err != nil {
		return result, fmt.Errorf("upload: %w", err)
	}

	result.Fid = entry.ID
	result.EncryptedSize = encSize
	return result, nil
}

func isZeroNonce(n [24]byte) bool {
	for _, b := range n {
		if b != 0 {
			return false
		}
	}
	return true
}

const partRetryMax = 3

func partRetryBackoff(attempt int) time.Duration {
	base := time.Duration(200<<uint(attempt)) * time.Millisecond
	jitter := float64(75+(attempt*11)%50) / 100.0
	return time.Duration(float64(base) * jitter)
}
