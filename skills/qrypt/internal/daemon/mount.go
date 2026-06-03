package daemon

import (
	"context"

	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
)

// mountBackend abstracts the FUSE filesystem lifecycle.
// Implementations:
//   - fuseBackend  (!nofuse build tag) — real FUSE mount
//   - noopBackend  (nofuse  build tag) — no-op stub
type mountBackend interface {
	mount(ctx context.Context, rc *config.ResolvedMountConfig, drv drive.Driver, cipher *crypt.RcloneCipher, cacheMgr *cache.CacheManager) error
	unmount() error
	VFS() CacheInvalidatable
}

// CacheInvalidatable allows the daemon to evict VFS cache entries.
type CacheInvalidatable interface {
	InvalidateDirCache(path string)
}

// RemoteRenamer allows the daemon to update the VFS node tree after
// a successful remote rename/move performed via the CLI path (qrypt mv).
type RemoteRenamer interface {
	OnRemoteRename(oldPath, newPath string)
}

// newMountBackend returns the appropriate backend for the current build.
// The implementation is chosen via build tags in mount_fuse.go / mount_nofuse.go.
func newMountBackend() mountBackend {
	return newPlatformMountBackend()
}
