package crypt

import (
	"fmt"
	"io"
)

// DecryptingReader streams plaintext bytes from an rclone-compatible encrypted reader.
// It assumes the File Header (magic + nonce) has already been read from the underlying stream,
// or that the caller is providing the exact block body stream.
type DecryptingReader struct {
	enc          io.Reader
	cipher       *RcloneCipher
	nonce        [24]byte
	blockIndex   uint64
	pending      []byte
	encEOF       bool
}

// NewDecryptingReader creates a new DecryptingReader.
// Note: This expects `enc` to provide the raw encrypted blocks, NOT the file header.
// The file header should be parsed out and passed as the `nonce`.
func NewDecryptingReader(enc io.Reader, cipher *RcloneCipher, nonce [24]byte) (*DecryptingReader, error) {
	return &DecryptingReader{
		enc:    enc,
		cipher: cipher,
		nonce:  nonce,
	}, nil
}

func (r *DecryptingReader) Read(p []byte) (int, error) {
	total := 0
	for total < len(p) {
		if len(r.pending) == 0 {
			if err := r.fillPending(); err != nil {
				if err == io.EOF && total > 0 {
					return total, nil
				}
				return total, err
			}
		}

		n := copy(p[total:], r.pending)
		total += n
		r.pending = r.pending[n:]
	}
	return total, nil
}

func (r *DecryptingReader) fillPending() error {
	if r.encEOF {
		return io.EOF
	}

	// Read a single encrypted block. It's either a full block or the last partial block.
	// Since we don't know the remaining size here reliably without tracking it from the caller,
	// we try to read a full BlockSize.
	encBlock := make([]byte, BlockSize)
	n, err := io.ReadFull(r.enc, encBlock)

	if err != nil {
		if err == io.EOF {
			if n == 0 {
				r.encEOF = true
				return io.EOF
			}
		} else if err != io.ErrUnexpectedEOF {
			return fmt.Errorf("read encrypted block: %w", err)
		}
	}

	if n == 0 {
		r.encEOF = true
		return io.EOF
	}

	// For the last block, n might be < BlockSize.
	encBlock = encBlock[:n]

	// Decrypt
	plaintext, err := r.cipher.DecryptBlock(encBlock, r.blockIndex, r.nonce)
	if err != nil {
		return fmt.Errorf("decrypt block %d: %w", r.blockIndex, err)
	}

	r.blockIndex++
	r.pending = plaintext

	return nil
}
