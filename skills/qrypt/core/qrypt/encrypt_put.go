package qrypt

import (
	"context"
	"io"
)

// EncryptPutRequest describes a one-shot streaming encrypt-and-upload.
// Used by FileAPI.Push (file-backed) and by FUSE writeback (staging-backed).
type EncryptPutRequest struct {
	// Reader streams the plain (unencrypted) bytes. Caller is responsible for closing.
	Reader io.Reader
	// PlainSize is the exact byte count of the plain stream. Required for backends
	// that need Content-Length up front.
	PlainSize int64
	// PlainName is the user-visible name. EncryptAndPut applies cipher.EncryptSegment
	// before calling up.Put.
	PlainName string
	// ParentID is the backend folder fid where the file will be stored.
	ParentID string
	// Nonce — set to a non-zero value to reuse a pre-existing nonce (e.g., for
	// upload resume). Zero value generates a fresh random nonce.
	Nonce [FileNonceSize]byte
}

// EncryptPutResult reports the outcome of EncryptAndPut.
type EncryptPutResult struct {
	// Entry is the backend-created entry (ID assigned by the cloud provider).
	Entry Entry
	// Nonce is the nonce that was actually used (either Request.Nonce if non-zero
	// or the freshly generated one).
	Nonce [FileNonceSize]byte
	// EncryptedSize is cipher.EncryptedSize(PlainSize).
	EncryptedSize int64
}

// EncryptAndPut performs streaming encryption + upload in a single pass. The
// canonical low-level "store a plain stream encrypted on the backend" primitive
// shared by every upload path (CLI push, mobile push, FUSE writeback).
//
// Callers must ensure up != nil and cp != nil; req.Reader and req.PlainSize
// must describe the same plain stream.
func EncryptAndPut(ctx context.Context, up Uploader, cp Cipher, req EncryptPutRequest) (EncryptPutResult, error) {
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

func isZeroNonce(n [FileNonceSize]byte) bool {
	for _, b := range n {
		if b != 0 {
			return false
		}
	}
	return true
}
