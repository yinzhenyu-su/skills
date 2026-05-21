//go:build nofuse

package daemon

import (
	"context"
	"fmt"

	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
)

type noopMountBackend struct{}

func newPlatformMountBackend() mountBackend {
	return &noopMountBackend{}
}

func (nb *noopMountBackend) mount(_ context.Context, _ *config.ResolvedMountConfig, _ drive.Driver, _ *crypt.RcloneCipher, _ *cache.CacheManager) error {
	return fmt.Errorf("FUSE not available in this build (build tag: nofuse)")
}

func (nb *noopMountBackend) unmount() error {
	return nil
}

func (nb *noopMountBackend) VFS() CacheInvalidatable {
	return nil
}
