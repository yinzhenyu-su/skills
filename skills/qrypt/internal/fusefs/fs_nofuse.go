//go:build nofuse

package fusefs

import (
	"context"
	"os/exec"
)

// QryptFS is a stub when FUSE is not available.
type QryptFS struct{}

// FSOptions is a stub when FUSE is not available.
type FSOptions struct {
	MaxRetries        int
	ConcurrentUploads int
	MemCacheSizeMB    int
}

// NewFS returns an error when FUSE is not available.
func NewFS(drv, cp, cacheMgr interface{}, rootFid string, opts FSOptions) *QryptFS {
	return &QryptFS{}
}

// Shutdown is a no-op when FUSE is not available.
func (*QryptFS) Shutdown() {}

// IsShuttingDown is a no-op when FUSE is not available.
func (*QryptFS) IsShuttingDown() bool { return false }

// UploadQueue is a stub when FUSE is not available.
type UploadQueue interface {
	Submit(job func(ctx context.Context) error) bool
}

// SetUploadQueue is a no-op when FUSE is not available.
func (*QryptFS) SetUploadQueue(q UploadQueue) {}

// MountOptions returns macOS mount options.
func MountOptions(allowOther bool, _ string) []string {
	opts := []string{"-o", "rw"}
	if allowOther {
		opts = append(opts, "-o", "allow_other")
	}
	return opts
}

// UnmountCommand returns the macOS unmount command.
func UnmountCommand(mountPoint string) *exec.Cmd {
	return exec.Command("umount", "-f", mountPoint)
}
