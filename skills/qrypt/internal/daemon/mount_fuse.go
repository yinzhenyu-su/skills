//go:build !nofuse

package daemon

import (
	"context"

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

func (fb *fuseMountBackend) mount(ctx context.Context, rc *config.ResolvedMountConfig, drv drive.Driver, cipher *crypt.RcloneCipher, cacheMgr *cache.CacheManager) error {
	rootFid := getRootFid(ctx, drv, rc)
	fb.vfs = fs.NewFS(drv, cipher, cacheMgr, rootFid, fs.FSOptions{
		MaxRetries:        rc.Sync.MaxRetries,
		ConcurrentUploads: rc.Sync.ConcurrentUploads,
		MemCacheSizeMB:    rc.Cache.MemCacheSizeMB,
	})

	fb.host = fuse.NewFileSystemHost(fb.vfs)
	options := fs.MountOptions(rc.AllowOther)

	go func() {
		fb.host.Mount(rc.MountPoint, options)
	}()

	log.L.Infof("fuse: mounted at %s\n", rc.MountPoint)
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
func getRootFid(ctx context.Context, drv drive.Driver, rc *config.ResolvedMountConfig) string {
	resolver, ok := drv.(interface {
		ResolvePath(ctx context.Context, path string) (string, error)
	})
	if !ok {
		return "0"
	}
	rootPath := config.RootPathForMount(config.MountInstance{
		Type:   rc.Type,
		Params: rc.Params,
	})
	if rootPath == "" || rootPath == "/" {
		return "0"
	}
	fid, err := resolver.ResolvePath(ctx, rootPath)
	if err != nil {
		return "0"
	}
	return fid
}
