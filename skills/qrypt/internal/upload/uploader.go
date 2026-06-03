package upload

import (
	"context"
	"fmt"
	"io"

	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
)

// Request describes a single FUSE-writeback upload. ParentFid, Name, PlainSize
// and DataReader are required. Nonce / OldFid / UploadID / LastPart support
// resumable / re-upload flows (not yet wired through but reserved for future use).
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

// Result reports the outcome of Upload. EncryptedSize comes from cipher.EncryptedSize.
type Result struct {
	Fid           string
	Nonce         [24]byte
	EncryptedSize int64
	PartCount     int
	UploadedBytes int64
	UploadID      string
}

// Uploader streams a staging-file body through the qrypt encrypt+put primitive.
// It is a FUSE-specific adapter; all CLI/mobile uploads go through qrypt.FileAPI.Push,
// which calls the same qrypt.EncryptAndPut underneath.
type Uploader struct {
	drv    qrypt.Driver
	cipher qrypt.Cipher
}

// NewUploader constructs an Uploader bound to a driver + cipher.
func NewUploader(drv qrypt.Driver, cipher qrypt.Cipher) *Uploader {
	return &Uploader{drv: drv, cipher: cipher}
}

// Upload opens the staging reader, encrypts the stream, and uploads in one pass.
// The underlying encrypt + put logic lives in qrypt.EncryptAndPut — shared with
// FileAPI.Push so all upload paths produce byte-identical results.
func (u *Uploader) Upload(ctx context.Context, req Request) (Result, error) {
	if req.DataReader == nil {
		return Result{}, fmt.Errorf("missing data reader for %s", req.Path)
	}

	up, ok := u.drv.(qrypt.Uploader)
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
