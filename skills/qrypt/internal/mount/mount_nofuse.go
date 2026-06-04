//go:build nofuse

package mount

import (
	"context"
	"fmt"

	"github.com/yinzhenyu/skills/qrypt/cipher"
	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

type noopMountBackend struct{}

func newPlatformMountBackend() mountBackend {
	return &noopMountBackend{}
}

func (nb *noopMountBackend) mount(_ context.Context, _ *config.ResolvedMountConfig, _ drivers.Driver, _ *cipher.RcloneCipher, _ *qrypt.CacheManager) error {
	return fmt.Errorf("FUSE not available in this build (build tag: nofuse)")
}

func (nb *noopMountBackend) unmount() error {
	return nil
}

func (nb *noopMountBackend) VFS() CacheInvalidatable {
	return nil
}
