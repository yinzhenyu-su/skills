package mount

import (
	"context"

	"github.com/yinzhenyu/skills/qrypt/internal/index"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/cipher"
	"github.com/yinzhenyu/skills/qrypt/internal/backend"
)

// mountBackend abstracts the FUSE filesystem lifecycle.
type mountBackend interface {
	mount(ctx context.Context, rc *config.ResolvedMountConfig, drv backend.Driver, cipher *cipher.RcloneCipher, cacheMgr *index.CacheManager) error
	unmount() error
	VFS() CacheInvalidatable
}

// CacheInvalidatable allows evicting VFS cache entries.
type CacheInvalidatable interface {
	InvalidateDirCache(path string)
}

// RemoteRenamer allows updating the VFS node tree after a successful remote rename/move.
type RemoteRenamer interface {
	OnRemoteRename(oldPath, newPath string)
}

func newMountBackend() mountBackend {
	return newPlatformMountBackend()
}
