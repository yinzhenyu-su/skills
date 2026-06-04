//go:build nofuse

package mount

import (
	"context"
	"fmt"

	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	"github.com/yinzhenyu/skills/qrypt/drivers"
)

type noopMountBackend struct{}

func newPlatformMountBackend() mountBackend {
	return &noopMountBackend{}
}

func (nb *noopMountBackend) mount(_ context.Context, _ *config.ResolvedMountConfig, _ drivers.Driver, _ *qrypt.RcloneCipher, _ *qrypt.CacheManager) error {
	return fmt.Errorf("FUSE not available in this build (build tag: nofuse)")
}

func (nb *noopMountBackend) unmount() error {
	return nil
}

func (nb *noopMountBackend) VFS() CacheInvalidatable {
	return nil
}
