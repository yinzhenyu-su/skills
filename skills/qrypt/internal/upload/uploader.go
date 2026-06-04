package upload

import (
	"context"
	"fmt"
	"io"

	"github.com/yinzhenyu/skills/qrypt/cipher"
	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	"github.com/yinzhenyu/skills/qrypt/drivers"
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
	drv    drivers.Driver
	cipher cipher.Cipher
}

func NewUploader(drv drivers.Driver, cipher cipher.Cipher) *Uploader {
	return &Uploader{drv: drv, cipher: cipher}
}

func (u *Uploader) Upload(ctx context.Context, req Request) (Result, error) {
	if req.DataReader == nil {
		return Result{}, fmt.Errorf("missing data reader for %s", req.Path)
	}

	up, ok := u.drv.(drivers.Uploader)
	if !ok {
		return Result{}, fmt.Errorf("driver does not support upload")
	}

	reader, err := req.DataReader()
	if err != nil {
		return Result{}, fmt.Errorf("open staging reader: %w", err)
	}
	defer reader.Close()

	out, err := qrypt.EncryptAndPut(ctx, up, u.cipher, qrypt.EncryptPutRequest{
		Reader:    reader,
		PlainSize: req.PlainSize,
		PlainName: req.Name,
		ParentID:  req.ParentFid,
		Nonce:     req.Nonce,
	})
	if err != nil {
		return Result{Nonce: out.Nonce, EncryptedSize: out.EncryptedSize}, fmt.Errorf("upload: %w", err)
	}

	return Result{
		Fid:           out.Entry.ID,
		Nonce:         out.Nonce,
		EncryptedSize: out.EncryptedSize,
	}, nil
}
