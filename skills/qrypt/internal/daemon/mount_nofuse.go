//go:build nofuse

package daemon

import (
	"context"
	"fmt"

	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
)

type noopMountBackend struct{}

func newPlatformMountBackend() mountBackend {
	return &noopMountBackend{}
}

func (nb *noopMountBackend) mount(_ context.Context, _ *config.Config, _ interface{}, _ *crypt.RcloneCipher, _ *cache.CacheManager) error {
	return fmt.Errorf("FUSE not available in this build (build tag: nofuse)")
}

func (nb *noopMountBackend) unmount() error {
	return nil
}
