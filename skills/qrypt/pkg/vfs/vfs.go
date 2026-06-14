// Package vfs defines the filesystem interface that the FUSE adapter and CLI
// commands can use. It uses plaintext paths visible to the user and avoids
// FUSE-specific types (Stat_t, errno) so it can be reused for other protocols.
package vfs

import (
	"context"
	"io"
	"time"
)

// NodeInfo contains the metadata for a filesystem node (file or directory).
type NodeInfo struct {
	ID      string    // fid (backend file identifier)
	Name    string    // plaintext filename
	IsDir   bool
	Size    int64     // plaintext file size (0 for directories)
	ModTime time.Time // last modification time
}

// DirEntry is a single entry returned by ListDir.
type DirEntry struct {
	Name  string
	IsDir bool
	Size  int64
}

// VFS defines the filesystem operations that the FUSE adapter and CLI commands
// can use. All paths are plaintext paths visible to the user.
//
// This interface intentionally avoids FUSE-specific types (Stat_t, errno) so it
// can be used across different protocols (FUSE, CLI, future WebDAV).
type VFS interface {
	// Read path
	Stat(ctx context.Context, path string) (NodeInfo, error)
	ListDir(ctx context.Context, path string) ([]DirEntry, error)
	Read(ctx context.Context, path string, offset int64) (io.ReadCloser, error)

	// Write path (synchronous operations)
	Create(ctx context.Context, path string) error
	Write(ctx context.Context, path string, offset int64, reader io.Reader) error
	Mkdir(ctx context.Context, path string) error
	Remove(ctx context.Context, path string) error
	Rename(ctx context.Context, oldPath, newPath string) error
	Truncate(ctx context.Context, path string, size int64) error
	Utimens(ctx context.Context, path string, mtime time.Time) error

	// Sync
	Flush(ctx context.Context, path string) error
	Fsync(ctx context.Context, path string) error

	// Lifecycle
	Shutdown()
}
