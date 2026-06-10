// Package drivers defines the storage backend interfaces and implementations.
//
// Driver, Writer, Uploader, PathResolver, and Cipher are the core contracts
// that every storage backend must implement. These types were originally
// defined in core/qrypt and are moved here so that drivers/ is self-contained.
package drivers

import (
	"context"
	"io"
	"time"

	"github.com/yinzhenyu/skills/qrypt/cipher"
)

// Entry is a single file-system entry returned by a storage backend.
type Entry struct {
	ID       string
	ParentID string
	Name     string
	IsDir    bool
	Size     int64
	ModTime  time.Time
	Extra    any
}

// Driver is the fundamental read-only storage backend interface.
type Driver interface {
	Init(ctx context.Context) error
	Drop(ctx context.Context) error
	List(ctx context.Context, parentID string) ([]Entry, error)
	Read(ctx context.Context, entry Entry, offset, size int64) (io.ReadCloser, error)
}

// Writer adds mutation operations for storage backends that support them.
type Writer interface {
	Mkdir(ctx context.Context, parentID, name string) (Entry, error)
	Move(ctx context.Context, entry Entry, dstParentID string) error
	Rename(ctx context.Context, entry Entry, newName string) error
	Remove(ctx context.Context, entry Entry) error
}

// BatchRemover is an optional interface for backends that can remove
// multiple entries in a single API call, avoiding N round-trips for
// bulk operations like recursive directory deletion.
type BatchRemover interface {
	BatchRemove(ctx context.Context, entries []Entry) error
}

// Uploader handles streaming uploads to the backend.
type Uploader interface {
	Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (Entry, error)
}

// PathResolver resolves a logical path to a backend-specific file identifier.
type PathResolver interface {
	ResolvePath(ctx context.Context, path string) (string, error)
}

// CipherSetter is implemented by drivers that support client-side encryption.
// After construction, callers should check for this interface and inject the cipher.
type CipherSetter interface {
	SetCipher(c cipher.Cipher)
}


type mtimeKey struct{}

// WithMtime attaches a modification time to the context for upload operations.
func WithMtime(ctx context.Context, mtime time.Time) context.Context {
	return context.WithValue(ctx, mtimeKey{}, mtime)
}

// MtimeFromContext extracts a modification time previously attached by WithMtime.
func MtimeFromContext(ctx context.Context) (time.Time, bool) {
	mtime, ok := ctx.Value(mtimeKey{}).(time.Time)
	return mtime, ok
}
