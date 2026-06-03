//go:build nofuse

package mount

import (
	"context"
	"fmt"

	"github.com/yinzhenyu/skills/qrypt/internal/index"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/cipher"
	"github.com/yinzhenyu/skills/qrypt/internal/backend"
)

type noopMountBackend struct{}

func newPlatformMountBackend() mountBackend {
	return &noopMountBackend{}
}

func (nb *noopMountBackend) mount(_ context.Context, _ *config.ResolvedMountConfig, _ backend.Driver, _ *cipher.RcloneCipher, _ *index.CacheManager) error {
	return fmt.Errorf("FUSE not available in this build (build tag: nofuse)")
}

func (nb *noopMountBackend) unmount() error {
	return nil
}

func (nb *noopMountBackend) VFS() CacheInvalidatable {
	return nil
}
