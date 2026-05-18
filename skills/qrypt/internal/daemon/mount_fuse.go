//go:build !nofuse

package daemon

import (
	"context"
	"fmt"

	"github.com/winfsp/cgofuse/fuse"

	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
	"github.com/yinzhenyu/skills/qrypt/internal/fs"
	"github.com/yinzhenyu/skills/qrypt/internal/log"
)

type fuseMountBackend struct {
	vfs  *fs.QryptFS
	host *fuse.FileSystemHost
}

func newPlatformMountBackend() mountBackend {
	return &fuseMountBackend{}
}

func (fb *fuseMountBackend) mount(ctx context.Context, cfg *config.Config, drv interface{}, cipher *crypt.RcloneCipher, cacheMgr *cache.CacheManager) error {
	driver, ok := drv.(drive.Driver)
	if !ok {
		return fmt.Errorf("driver does not implement drive.Driver")
	}

	rootFid := getRootFid(ctx, driver, cfg)
	fb.vfs = fs.NewFS(driver, cipher, cacheMgr, rootFid, fs.FSOptions{
		MaxRetries:        cfg.Sync.MaxRetries,
		ConcurrentUploads: cfg.Sync.ConcurrentUploads,
		MemCacheSizeMB:    cfg.Cache.MemCacheSizeMB,
	})

	fb.host = fuse.NewFileSystemHost(fb.vfs)
	options := fs.MountOptions(cfg.Mount.AllowOther)

	go func() {
		fb.host.Mount(cfg.Mount.Point, options)
	}()

	log.L.Infof("fuse: mounted at %s\n", cfg.Mount.Point)
	return nil
}

func (fb *fuseMountBackend) unmount() error {
	if fb.vfs != nil {
		fb.vfs.Shutdown()
	}
	if fb.host != nil {
		fb.host.Unmount()
	}
	return nil
}

// getRootFid resolves the root path to a FID.
func getRootFid(ctx context.Context, drv drive.Driver, cfg *config.Config) string {
	resolver, ok := drv.(interface {
		ResolvePath(ctx context.Context, path string) (string, error)
	})
	if !ok {
		return "0"
	}
	rootPath := cfg.RootPath()
	if rootPath == "" || rootPath == "/" {
		return "0"
	}
	fid, err := resolver.ResolvePath(ctx, rootPath)
	if err != nil {
		return "0"
	}
	return fid
}
