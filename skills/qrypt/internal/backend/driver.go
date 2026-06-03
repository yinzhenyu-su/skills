// Package drive defines a composable storage backend interface for qrypt.
// Inspired by Alist's composable driver pattern (alistgo/alist/internal/driver).
//
// A Driver (Meta + Reader) is the minimum requirement for read-only access.
// Optional Writer and Uploader interfaces enable write operations.
// Consumers use type assertions to discover capabilities:
//
//	if w, ok := drv.(backend.Writer); ok { w.Mkdir(...) }
package backend

import (
	"context"
	"io"
	"time"
)

// PathResolver is the interface for resolving remote paths to FIDs.
type PathResolver interface {
	ResolvePath(ctx context.Context, path string) (string, error)
}

// Entry is a universal file/directory descriptor returned by all drivers.
type Entry struct {
	ID       string
	ParentID string // optional, used by callers for cache invalidation on write ops
	Name     string
	IsDir    bool
	Size     int64
	ModTime  time.Time
	// Extra carries driver-specific metadata transparently.
	// Consumers may type-assert to access implementation details.
	Extra any
}

// Meta is the lifecycle interface every driver must implement.
type Meta interface {
	// Init initialises the driver (auth, session setup, etc.).
	// Called once at mount time.
	Init(ctx context.Context) error

	// Drop tears down the driver (release resources, flush state, etc.).
	Drop(ctx context.Context) error
}

// Reader is the read-only operation interface.
// Every driver must implement Reader.
type Reader interface {
	// List returns child entries of the directory identified by parentID.
	List(ctx context.Context, parentID string) ([]Entry, error)

	// Read returns an io.ReadCloser for the byte range [offset, offset+size)
	// of the given entry. The caller MUST close the reader after use.
	Read(ctx context.Context, entry Entry, offset, size int64) (io.ReadCloser, error)
}

// Writer is the optional mutation interface.
type Writer interface {
	// Mkdir creates a new directory under parentID and returns its entry.
	Mkdir(ctx context.Context, parentID, name string) (Entry, error)

	// Move moves entry to a different parent directory.
	Move(ctx context.Context, entry Entry, dstParentID string) error

	// Rename renames entry within the same parent.
	Rename(ctx context.Context, entry Entry, newName string) error

	// Remove deletes a file or directory entry.
	Remove(ctx context.Context, entry Entry) error
}

// Uploader is the optional upload interface.
type Uploader interface {
	// Put uploads a file's content (reader, size) to the given parent directory.
	// Returns the created entry on success.
	Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (Entry, error)
}

// Driver combines the required Meta and Reader interfaces.
// A Driver may optionally implement Writer and/or Uploader.
type Driver interface {
	Meta
	Reader
}

type mtimeKey struct{}

func WithMtime(ctx context.Context, mtime time.Time) context.Context {
	return context.WithValue(ctx, mtimeKey{}, mtime)
}

func MtimeFromContext(ctx context.Context) (time.Time, bool) {
	mtime, ok := ctx.Value(mtimeKey{}).(time.Time)
	return mtime, ok
}
