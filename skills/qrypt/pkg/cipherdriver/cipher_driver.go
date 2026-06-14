// Package cipherdriver provides a transparent-encryption wrapper around a raw
// drivers.Driver. It encrypts names on the way down (Mkdir, Rename, Put) and
// decrypts names on the way up (List). Content encryption/decryption is handled
// at the block level through ReadBlock, while Read passes ciphertext through
// to the caller for streaming decryption.
package cipherdriver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/yinzhenyu/skills/qrypt/cipher"
	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
)

// CipherDriver wraps a raw drivers.Driver with transparent name and block
// encryption, implementing drivers.Driver, drivers.Writer, and drivers.Uploader.
type CipherDriver struct {
	raw        drivers.Driver
	cp         cipher.Cipher
	nonceCache sync.Map // fid → [cipher.FileNonceSize]byte
}

// New creates a CipherDriver that encrypts/decrypts data through raw using cp.
func New(raw drivers.Driver, cp cipher.Cipher) *CipherDriver {
	return &CipherDriver{
		raw: raw,
		cp:  cp,
	}
}

// --------------------------------------------------------------------------
// drivers.Driver — pass-through / name-decrypting
// --------------------------------------------------------------------------

// Init delegates to the raw driver.
func (cd *CipherDriver) Init(ctx context.Context) error {
	return cd.raw.Init(ctx)
}

// Drop delegates to the raw driver.
func (cd *CipherDriver) Drop(ctx context.Context) error {
	return cd.raw.Drop(ctx)
}

// List returns directory entries with decrypted names. Entries whose names
// cannot be decrypted are logged and skipped.
func (cd *CipherDriver) List(ctx context.Context, parentID string) ([]drivers.Entry, error) {
	entries, err := cd.raw.List(ctx, parentID)
	if err != nil {
		return nil, err
	}

	decrypted := make([]drivers.Entry, 0, len(entries))
	for _, e := range entries {
		decName, err := cd.cp.DecryptSegment(e.Name)
		if err != nil {
			logging.L.Warnf("cipherdriver: decrypt name %q: %v", e.Name, err)
			continue
		}
		e.Name = strings.TrimSpace(decName)
		decrypted = append(decrypted, e)
	}
	return decrypted, nil
}

// Read passes through to the raw driver, returning the raw ciphertext stream.
// Callers must use ReadBlock or a DecryptingReader to obtain plaintext.
func (cd *CipherDriver) Read(ctx context.Context, entry drivers.Entry, offset, size int64) (io.ReadCloser, error) {
	return cd.raw.Read(ctx, entry, offset, size)
}

// --------------------------------------------------------------------------
// drivers.Writer — name-encrypting mutations
// --------------------------------------------------------------------------

// writer returns the raw driver as a Writer, or an error if unsupported.
func (cd *CipherDriver) writer() (drivers.Writer, error) {
	w, ok := cd.raw.(drivers.Writer)
	if !ok {
		return nil, errors.New("driver does not support write operations")
	}
	return w, nil
}

// Mkdir creates a directory with an encrypted name on the backend.
func (cd *CipherDriver) Mkdir(ctx context.Context, parentID, name string) (drivers.Entry, error) {
	w, err := cd.writer()
	if err != nil {
		return drivers.Entry{}, err
	}
	return w.Mkdir(ctx, parentID, cd.cp.EncryptSegment(name))
}

// Move delegates to the raw writer without name encryption — the target
// parentID is already a backend identifier.
func (cd *CipherDriver) Move(ctx context.Context, entry drivers.Entry, dstParentID string) error {
	w, err := cd.writer()
	if err != nil {
		return err
	}
	return w.Move(ctx, entry, dstParentID)
}

// Rename renames an entry with an encrypted name on the backend.
func (cd *CipherDriver) Rename(ctx context.Context, entry drivers.Entry, newName string) error {
	w, err := cd.writer()
	if err != nil {
		return err
	}
	return w.Rename(ctx, entry, cd.cp.EncryptSegment(newName))
}

// Remove delegates to the raw writer directly (fid-based).
func (cd *CipherDriver) Remove(ctx context.Context, entry drivers.Entry) error {
	w, err := cd.writer()
	if err != nil {
		return err
	}
	return w.Remove(ctx, entry)
}

// BatchRemove delegates to the raw driver's BatchRemover if available,
// otherwise falls back to per-entry Remove.
func (cd *CipherDriver) BatchRemove(ctx context.Context, entries []drivers.Entry) error {
	if br, ok := cd.raw.(drivers.BatchRemover); ok {
		return br.BatchRemove(ctx, entries)
	}
	for _, entry := range entries {
		if err := cd.Remove(ctx, entry); err != nil {
			return err
		}
	}
	return nil
}

// --------------------------------------------------------------------------
// drivers.Uploader — streaming encrypt-and-upload
// --------------------------------------------------------------------------

// uploader returns the raw driver as an Uploader, or an error if unsupported.
func (cd *CipherDriver) uploader() (drivers.Uploader, error) {
	u, ok := cd.raw.(drivers.Uploader)
	if !ok {
		return nil, errors.New("driver does not support upload operations")
	}
	return u, nil
}

// Put encrypts the plaintext body on the fly and uploads it to the backend
// with an encrypted name. A random file nonce is generated and prepended in
// the header so it can be recovered during ReadBlock.
func (cd *CipherDriver) Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (drivers.Entry, error) {
	u, err := cd.uploader()
	if err != nil {
		return drivers.Entry{}, err
	}

	nonce, err := cd.cp.GenerateRandomNonce()
	if err != nil {
		return drivers.Entry{}, fmt.Errorf("generate nonce: %w", err)
	}

	encName := cd.cp.EncryptSegment(name)
	encSize := cd.cp.EncryptedSize(size)
	encReader := cipher.NewEncryptingReader(body, cd.cp, nonce, size)

	entry, err := u.Put(ctx, parentID, encName, encSize, encReader)
	if err != nil {
		return drivers.Entry{}, err
	}

	// Cache the nonce so future ReadBlock calls can avoid re-reading the header.
	cd.nonceCache.Store(entry.ID, nonce)

	return entry, nil
}

// --------------------------------------------------------------------------
// Block-level decryption (extra method)
// --------------------------------------------------------------------------

// ReadBlock decrypts a single block at blockIdx and returns the plaintext.
// The file nonce is cached internally after the first block read for a given
// file so the header is only fetched once per fid.
func (cd *CipherDriver) ReadBlock(ctx context.Context, entry drivers.Entry, blockIdx uint64) ([]byte, error) {
	nonce, err := cd.getNonce(ctx, entry)
	if err != nil {
		return nil, err
	}

	offset := int64(cipher.FileHeaderSize) + int64(blockIdx)*int64(cipher.BlockSize)

	rc, err := cd.raw.Read(ctx, entry, offset, int64(cipher.BlockSize))
	if err != nil {
		return nil, fmt.Errorf("read encrypted block: %w", err)
	}
	defer rc.Close()

	ciphertext, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read encrypted block data: %w", err)
	}

	plaintext, err := cd.cp.DecryptBlock(ciphertext, blockIdx, nonce)
	if err != nil {
		return nil, fmt.Errorf("decrypt block %d: %w", blockIdx, err)
	}
	return plaintext, nil
}

// getNonce returns the file nonce for entry, reading the file header once and
// caching the result in cd.nonceCache.
func (cd *CipherDriver) getNonce(ctx context.Context, entry drivers.Entry) ([cipher.FileNonceSize]byte, error) {
	if cached, ok := cd.nonceCache.Load(entry.ID); ok {
		return cached.([cipher.FileNonceSize]byte), nil
	}

	rc, err := cd.raw.Read(ctx, entry, 0, int64(cipher.FileHeaderSize))
	if err != nil {
		return [cipher.FileNonceSize]byte{}, fmt.Errorf("read file header: %w", err)
	}
	defer rc.Close()

	header, err := io.ReadAll(rc)
	if err != nil {
		return [cipher.FileNonceSize]byte{}, fmt.Errorf("read header data: %w", err)
	}
	if len(header) < cipher.FileHeaderSize {
		return [cipher.FileNonceSize]byte{}, fmt.Errorf("header too short: %d bytes", len(header))
	}

	if !bytes.Equal(header[:cipher.FileMagicSize], []byte(cipher.FileMagic)) {
		return [cipher.FileNonceSize]byte{}, errors.New("invalid file magic")
	}

	var nonce [cipher.FileNonceSize]byte
	copy(nonce[:], header[cipher.FileMagicSize:cipher.FileHeaderSize])
	cd.nonceCache.Store(entry.ID, nonce)

	return nonce, nil
}
