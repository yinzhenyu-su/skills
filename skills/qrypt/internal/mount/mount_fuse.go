//go:build !nofuse

package mount

import (
	"context"
	"time"

	"github.com/winfsp/cgofuse/fuse"

	"github.com/yinzhenyu/skills/qrypt/cipher"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/internal/fusefs"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
)

type fuseMountBackend struct {
	vfs  *fusefs.QryptFS
	host *fuse.FileSystemHost
}

func newPlatformMountBackend() mountBackend {
	return &fuseMountBackend{}
}

func (fb *fuseMountBackend) mount(ctx context.Context, rc *config.ResolvedMountConfig, drv drivers.Driver, cp *cipher.RcloneCipher, cacheMgr *qrypt.CacheManager) error {
	rootFid := getRootFid(ctx, drv, rc)

	writeBackDelay, _ := time.ParseDuration(rc.Sync.WriteBackTimeout)

	fb.vfs = fusefs.NewFS(drv, cp, cacheMgr, rootFid, fusefs.FSOptions{
		MaxRetries:        rc.Sync.MaxRetries,
		ConcurrentUploads: rc.Sync.ConcurrentUploads,
		MemCacheSizeMB:    rc.Cache.MemCacheSizeMB,
		WriteBackTimeout:  writeBackDelay,
	})

	fb.host = fuse.NewFileSystemHost(fb.vfs)
	options := fusefs.MountOptions(rc.AllowOther, rc.VolName)

	go func() {
		fb.host.Mount(rc.MountPoint, options)
	}()

	logging.L.Infof("fuse: mounted at %s\n", rc.MountPoint)
	return nil
}

func (fb *fuseMountBackend) VFS() CacheInvalidatable {
	return fb.vfs
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

func getRootFid(ctx context.Context, drv drivers.Driver, rc *config.ResolvedMountConfig) string {
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
