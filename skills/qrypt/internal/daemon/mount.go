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
}

// newMountBackend returns the appropriate backend for the current build.
// The implementation is chosen via build tags in mount_fuse.go / mount_nofuse.go.
func newMountBackend() mountBackend {
	return newPlatformMountBackend()
}
