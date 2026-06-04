package qrypt

import (
	"context"
	"io"

	"github.com/yinzhenyu/skills/qrypt/cipher"
	"github.com/yinzhenyu/skills/qrypt/drivers"
)

// EncryptPutRequest describes a one-shot streaming encrypt-and-upload.
type EncryptPutRequest struct {
	Reader    io.Reader
	PlainSize int64
	PlainName string
	ParentID  string
	Nonce     [cipher.FileNonceSize]byte
}

// EncryptPutResult reports the outcome of EncryptAndPut.
type EncryptPutResult struct {
	Entry         drivers.Entry
	Nonce         [cipher.FileNonceSize]byte
	EncryptedSize int64
}

func EncryptAndPut(ctx context.Context, up drivers.Uploader, cp cipher.Cipher, req EncryptPutRequest) (EncryptPutResult, error) {
	nonce := req.Nonce
	if isZeroNonce(nonce) {
		var err error
		nonce, err = cp.GenerateRandomNonce()
		if err != nil {
			return EncryptPutResult{}, WrapError(ErrInternal, "generate nonce", err)
		}
	}

	encName := cp.EncryptSegment(req.PlainName)
	encSize := cp.EncryptedSize(req.PlainSize)
	encReader := NewEncryptingReader(req.Reader, cp, nonce, req.PlainSize)

	entry, err := up.Put(ctx, req.ParentID, encName, encSize, encReader)
	if err != nil {
		return EncryptPutResult{Nonce: nonce, EncryptedSize: encSize}, err
	}
	return EncryptPutResult{Entry: entry, Nonce: nonce, EncryptedSize: encSize}, nil
}

func isZeroNonce(n [cipher.FileNonceSize]byte) bool {
	for _, b := range n {
		if b != 0 {
			return false
		}
	}
	return true
}
