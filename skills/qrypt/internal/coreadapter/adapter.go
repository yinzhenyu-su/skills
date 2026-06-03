// Package coreadapter bridges internal/backend + internal/cipher (concrete implementations)
// to core/qrypt interfaces (Driver, Writer, Uploader, PathResolver, Cipher).
//
// This is L1 — it depends on L0 (core/qrypt) and L1 (internal/backend, internal/cipher),
// and is depended on by every L2 adapter (cmd, daemon, mobile, platform) plus L1 peers
// (fusefs, mount) that need to bridge backend types into the kernel's interface contracts.
//
// One adapter, one place. Replaces 3 prior copies that lived in platform/, fusefs/, and
// daemon/.
package coreadapter

import (
	"context"
	"fmt"
	"io"

	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	"github.com/yinzhenyu/skills/qrypt/internal/backend"
	"github.com/yinzhenyu/skills/qrypt/internal/cipher"
)

// DriverAdapter wraps a backend.Driver and satisfies qrypt.Driver, qrypt.Writer,
// qrypt.Uploader, qrypt.PathResolver via type assertion. Methods that the inner
// driver does not support return ErrUnsupported.
//
// Inner() exposes the wrapped backend.Driver for callers that need backend-specific
// features (e.g. SetCipher) not part of the kernel interface contract.
type DriverAdapter struct {
	inner backend.Driver
}

// NewDriverAdapter constructs a DriverAdapter.
func NewDriverAdapter(inner backend.Driver) *DriverAdapter {
	return &DriverAdapter{inner: inner}
}

// Inner returns the wrapped backend.Driver.
func (a *DriverAdapter) Inner() backend.Driver { return a.inner }

func (a *DriverAdapter) Init(ctx context.Context) error { return a.inner.Init(ctx) }
func (a *DriverAdapter) Drop(ctx context.Context) error { return a.inner.Drop(ctx) }

func (a *DriverAdapter) List(ctx context.Context, parentID string) ([]qrypt.Entry, error) {
	entries, err := a.inner.List(ctx, parentID)
	if err != nil {
		return nil, err
	}
	out := make([]qrypt.Entry, len(entries))
	for i, e := range entries {
		out[i] = ToCoreEntry(e)
	}
	return out, nil
}

func (a *DriverAdapter) Read(ctx context.Context, entry qrypt.Entry, offset, size int64) (io.ReadCloser, error) {
	return a.inner.Read(ctx, ToBackendEntry(entry), offset, size)
}

func (a *DriverAdapter) Mkdir(ctx context.Context, parentID, name string) (qrypt.Entry, error) {
	w, ok := a.inner.(backend.Writer)
	if !ok {
		return qrypt.Entry{}, fmt.Errorf("driver does not support write operations")
	}
	e, err := w.Mkdir(ctx, parentID, name)
	return ToCoreEntry(e), err
}

func (a *DriverAdapter) Move(ctx context.Context, entry qrypt.Entry, dstParentID string) error {
	w, ok := a.inner.(backend.Writer)
	if !ok {
		return fmt.Errorf("driver does not support move")
	}
	return w.Move(ctx, ToBackendEntry(entry), dstParentID)
}

func (a *DriverAdapter) Rename(ctx context.Context, entry qrypt.Entry, newName string) error {
	w, ok := a.inner.(backend.Writer)
	if !ok {
		return fmt.Errorf("driver does not support rename")
	}
	return w.Rename(ctx, ToBackendEntry(entry), newName)
}

func (a *DriverAdapter) Remove(ctx context.Context, entry qrypt.Entry) error {
	w, ok := a.inner.(backend.Writer)
	if !ok {
		return fmt.Errorf("driver does not support delete")
	}
	return w.Remove(ctx, ToBackendEntry(entry))
}

func (a *DriverAdapter) Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (qrypt.Entry, error) {
	up, ok := a.inner.(backend.Uploader)
	if !ok {
		return qrypt.Entry{}, fmt.Errorf("driver does not support upload")
	}
	e, err := up.Put(ctx, parentID, name, size, body)
	return ToCoreEntry(e), err
}

func (a *DriverAdapter) ResolvePath(ctx context.Context, path string) (string, error) {
	r, ok := a.inner.(backend.PathResolver)
	if !ok {
		return "", fmt.Errorf("driver does not support path resolution")
	}
	return r.ResolvePath(ctx, path)
}

// CipherAdapter wraps a *cipher.RcloneCipher to satisfy qrypt.Cipher.
type CipherAdapter struct {
	inner *cipher.RcloneCipher
}

// NewCipherAdapter constructs a CipherAdapter.
func NewCipherAdapter(c *cipher.RcloneCipher) CipherAdapter {
	return CipherAdapter{inner: c}
}

// Inner returns the wrapped cipher.
func (a CipherAdapter) Inner() *cipher.RcloneCipher { return a.inner }

func (a CipherAdapter) EncryptSegment(plain string) string {
	return a.inner.EncryptSegment(plain)
}

func (a CipherAdapter) DecryptSegment(cipher string) (string, error) {
	return a.inner.DecryptSegment(cipher)
}

func (a CipherAdapter) EncryptBlock(plaintext []byte, blockIndex uint64, nonce [qrypt.FileNonceSize]byte) ([]byte, error) {
	return a.inner.EncryptBlock(plaintext, blockIndex, nonce)
}

func (a CipherAdapter) DecryptBlock(ciphertext []byte, blockIndex uint64, nonce [qrypt.FileNonceSize]byte) ([]byte, error) {
	return a.inner.DecryptBlock(ciphertext, blockIndex, nonce)
}

func (a CipherAdapter) EncryptedSize(plainSize int64) int64 {
	return a.inner.EncryptedSize(plainSize)
}

func (a CipherAdapter) DecryptedSize(cipherSize int64) (int64, error) {
	return a.inner.DecryptedSize(cipherSize)
}

func (a CipherAdapter) GenerateRandomNonce() ([qrypt.FileNonceSize]byte, error) {
	return a.inner.GenerateRandomNonce()
}

// singleDriverFactory wraps a single backend.Driver as a qrypt.DriverFactory.
// Suitable for cases where only one driver is needed (CLI, mobile single mount).
type singleDriverFactory struct {
	inner backend.Driver
}

// NewSingleDriverFactory returns a DriverFactory that always yields the same wrapped driver.
func NewSingleDriverFactory(drv backend.Driver) qrypt.DriverFactory {
	return &singleDriverFactory{inner: drv}
}

func (f *singleDriverFactory) CreateDriver(ctx context.Context, cfg qrypt.SessionConfig) (qrypt.Driver, error) {
	return NewDriverAdapter(f.inner), nil
}

// ToCoreEntry converts a backend.Entry to qrypt.Entry.
func ToCoreEntry(e backend.Entry) qrypt.Entry {
	return qrypt.Entry{
		ID:      e.ID,
		Name:    e.Name,
		IsDir:   e.IsDir,
		Size:    e.Size,
		ModTime: e.ModTime,
	}
}

// ToBackendEntry converts a qrypt.Entry to backend.Entry.
func ToBackendEntry(e qrypt.Entry) backend.Entry {
	return backend.Entry{
		ID:      e.ID,
		Name:    e.Name,
		IsDir:   e.IsDir,
		Size:    e.Size,
		ModTime: e.ModTime,
	}
}
