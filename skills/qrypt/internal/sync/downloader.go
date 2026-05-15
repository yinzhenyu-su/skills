package sync

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
)

type DownloadRequest struct {
	Entry      drive.Entry
	LocalPath  string
	ProgressFn func(written int64, total int64)
}

type Downloader struct {
	drv    drive.Driver
	cipher *crypt.RcloneCipher
}

func NewDownloader(drv drive.Driver, cipher *crypt.RcloneCipher) *Downloader {
	return &Downloader{drv: drv, cipher: cipher}
}

type progressWriter struct {
	written int64
	total   int64
	fn      func(int64, int64)
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.written += int64(n)
	if pw.fn != nil {
		pw.fn(pw.written, pw.total)
	}
	return n, nil
}

func (d *Downloader) Download(ctx context.Context, req DownloadRequest) error {
	outFile, err := os.Create(req.LocalPath)
	if err != nil {
		return fmt.Errorf("create local file: %w", err)
	}
	
	success := false
	defer func() {
		outFile.Close()
		if !success {
			os.Remove(req.LocalPath)
		}
	}()

	encSize := req.Entry.Size
	bodySize, err := d.cipher.DecryptedSize(encSize)
	if err != nil {
		return fmt.Errorf("invalid file size: %w", err)
	}

	headerSize := int64(crypt.FileHeaderSize)
	rcHeader, err := d.drv.Read(ctx, req.Entry, 0, headerSize)
	if err != nil {
		return fmt.Errorf("read header: %w", err)
	}
	header := make([]byte, headerSize)
	if _, err := io.ReadFull(rcHeader, header); err != nil {
		rcHeader.Close()
		return fmt.Errorf("read header data: %w", err)
	}
	rcHeader.Close()

	var fileNonce [crypt.FileNonceSize]byte
	copy(fileNonce[:], header[crypt.FileMagicSize:])

	encBodySize := encSize - headerSize
	if encBodySize <= 0 {
		return nil
	}

	// Single read request for the entire body
	rcBody, err := d.drv.Read(ctx, req.Entry, headerSize, encBodySize)
	if err != nil {
		return fmt.Errorf("read body stream: %w", err)
	}
	defer rcBody.Close()

	decReader, err := crypt.NewDecryptingReader(rcBody, d.cipher, fileNonce)
	if err != nil {
		return fmt.Errorf("create decrypting reader: %w", err)
	}

	var dst io.Writer = outFile
	if req.ProgressFn != nil {
		dst = io.MultiWriter(outFile, &progressWriter{total: bodySize, fn: req.ProgressFn})
	}

	_, err = io.Copy(dst, decReader)
	if err != nil {
		return fmt.Errorf("copy decrypted stream: %w", err)
	}

	success = true
	return nil
}
