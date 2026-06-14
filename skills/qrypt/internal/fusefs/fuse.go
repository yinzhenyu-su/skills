//go:build !nofuse

package fusefs

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/winfsp/cgofuse/fuse"

	"github.com/yinzhenyu/skills/qrypt/internal/logging"
	vfspkg "github.com/yinzhenyu/skills/qrypt/pkg/vfs"
)

type UploadQueue interface {
	Submit(job func(ctx context.Context) error) bool
}

// FUSEAdapter translates cgofuse FileSystemBase callbacks into VFS interface
// calls. It is a thin adapter that owns no business logic — all filesystem
// operations are delegated to the underlying VFS.
//
// FUSEAdapter embeds fuse.FileSystemBase for default no-op implementations of
// callbacks not explicitly handled (Chmod, Chown, xattr, etc.).
type FUSEAdapter struct {
	fuse.FileSystemBase
	vfs          vfspkg.VFS
	shuttingDown int32
	fhCounter    uint64
}

// NewFUSEAdapter creates a new FUSEAdapter backed by the given VFS.
func NewFUSEAdapter(vfs vfspkg.VFS) *FUSEAdapter {
	return &FUSEAdapter{vfs: vfs}
}

func (fa *FUSEAdapter) SetUploadQueue(q UploadQueue) {}

// IsShuttingDown reports whether shutdown has been requested.
func (fa *FUSEAdapter) IsShuttingDown() bool {
	return atomic.LoadInt32(&fa.shuttingDown) == 1
}

func (fa *FUSEAdapter) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	start := time.Now()
	defer logFuseResult("Getattr", path, start, &errc)

	info, err := fa.vfs.Stat(context.Background(), path)
	if err != nil {
		return -fuse.ENOENT
	}

	uid, gid, _ := fuse.Getcontext()
	stat.Uid = uid
	stat.Gid = gid

	if info.IsDir {
		stat.Mode = fuse.S_IFDIR | 0755
		stat.Nlink = 2
	} else {
		stat.Mode = fuse.S_IFREG | 0644
		stat.Nlink = 1
		stat.Size = info.Size
	}
	stat.Mtim = fuse.NewTimespec(info.ModTime)
	stat.Atim = stat.Mtim
	stat.Ctim = stat.Mtim
	return 0
}

func (fa *FUSEAdapter) Access(path string, mask uint32) (errc int) {
	_, err := fa.vfs.Stat(context.Background(), path)
	if err != nil {
		return -fuse.ENOENT
	}
	return 0
}

func (fa *FUSEAdapter) Readdir(path string, fill func(name string, stat *fuse.Stat_t, ofst int64) bool, ofst int64, fh uint64) (errc int) {
	start := time.Now()
	defer logFuseResult("Readdir", path, start, &errc)

	fill(".", nil, 0)
	fill("..", nil, 0)

	entries, err := fa.vfs.ListDir(context.Background(), path)
	if err != nil {
		return -fuse.EIO
	}

	uid, gid, _ := fuse.Getcontext()
	for _, e := range entries {
		stat := &fuse.Stat_t{}
		stat.Uid = uid
		stat.Gid = gid
		if e.IsDir {
			stat.Mode = fuse.S_IFDIR | 0755
			stat.Nlink = 2
		} else {
			stat.Mode = fuse.S_IFREG | 0644
			stat.Nlink = 1
			stat.Size = e.Size
		}
		fill(e.Name, stat, 0)
	}
	return 0
}

func (fa *FUSEAdapter) Read(path string, buff []byte, ofst int64, fh uint64) (n int) {
	start := time.Now()

	rc, err := fa.vfs.Read(context.Background(), path, ofst)
	if err != nil {
		return 0
	}
	defer rc.Close()

	total, _ := io.ReadFull(rc, buff)
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		logging.L.Warnf("[FUSE] Read slow: path=%s offset=%d len=%d -> %dB in %v (%.1f KB/s)\n",
			path, ofst, len(buff), total, elapsed, float64(total)/elapsed.Seconds()/1024)
	}
	return total
}

func (fa *FUSEAdapter) Write(path string, buff []byte, ofst int64, fh uint64) (n int) {
	if fa.IsShuttingDown() {
		return 0
	}

	err := fa.vfs.Write(context.Background(), path, ofst, bytes.NewReader(buff))
	if err != nil {
		return 0
	}
	return len(buff)
}

func (fa *FUSEAdapter) Create(path string, flags int, mode uint32) (errc int, fh uint64) {
	start := time.Now()
	defer func() {
		logging.L.Debugf("[TIMER] Create(%s): took %v\n", path, time.Since(start))
	}()

	// Handle macOS Finder drag-drop directory creation: if there's no
	// dot in the filename and it doesn't start with a dot, treat as Mkdir.
	name := filepath.Base(path)
	if !strings.Contains(name, ".") && !strings.HasPrefix(name, ".") {
		if errc = fa.Mkdir(path, mode); errc != 0 {
			return errc, 0
		}
		return 0, 0
	}

	err := fa.vfs.Create(context.Background(), path)
	if err != nil {
		return -fuse.EIO, 0
	}
	return 0, atomic.AddUint64(&fa.fhCounter, 1)
}

func (fa *FUSEAdapter) Mkdir(path string, mode uint32) (errc int) {
	start := time.Now()
	defer logFuseResult("Mkdir", path, start, &errc)
	if fa.IsShuttingDown() {
		return -fuse.EIO
	}

	err := fa.vfs.Mkdir(context.Background(), path)
	if err != nil {
		return -fuse.EIO
	}
	return 0
}

func (fa *FUSEAdapter) Unlink(path string) (errc int) {
	start := time.Now()
	defer logFuseResult("Unlink", path, start, &errc)
	if fa.IsShuttingDown() {
		return -fuse.EIO
	}

	err := fa.vfs.Remove(context.Background(), path)
	if err != nil {
		return -fuse.EIO
	}
	return 0
}

func (fa *FUSEAdapter) Rmdir(path string) (errc int) {
	start := time.Now()
	defer logFuseResult("Rmdir", path, start, &errc)
	if fa.IsShuttingDown() {
		return -fuse.EIO
	}

	err := fa.vfs.Remove(context.Background(), path)
	if err != nil {
		return -fuse.EIO
	}
	return 0
}

func (fa *FUSEAdapter) Rename(oldPath string, newPath string) (errc int) {
	return fa.Rename3(oldPath, newPath, 0)
}

func (fa *FUSEAdapter) Rename3(oldPath string, newPath string, flags uint32) (errc int) {
	start := time.Now()
	defer logFuseResult("Rename", oldPath+" -> "+newPath, start, &errc)
	if fa.IsShuttingDown() {
		return -fuse.EIO
	}

	err := fa.vfs.Rename(context.Background(), oldPath, newPath)
	if err != nil {
		return -fuse.EIO
	}
	return 0
}

func (fa *FUSEAdapter) Truncate(path string, size int64, fh uint64) (errc int) {
	if fa.IsShuttingDown() {
		return -fuse.EIO
	}

	err := fa.vfs.Truncate(context.Background(), path, size)
	if err != nil {
		return -fuse.EIO
	}
	return 0
}

func (fa *FUSEAdapter) Utimens(path string, tmsp []fuse.Timespec) (errc int) {
	if len(tmsp) > 1 {
		err := fa.vfs.Utimens(context.Background(), path, tmsp[1].Time())
		if err != nil {
			return -fuse.EIO
		}
	}
	return 0
}

func (fa *FUSEAdapter) Flush(path string, fh uint64) (errc int) {
	return 0
}

func (fa *FUSEAdapter) Fsync(path string, datasync bool, fh uint64) (errc int) {
	return 0
}

func (fa *FUSEAdapter) Open(path string, flags int) (errc int, fh uint64) {
	_, err := fa.vfs.Stat(context.Background(), path)
	if err != nil {
		return -fuse.ENOENT, 0
	}
	return 0, atomic.AddUint64(&fa.fhCounter, 1)
}

func (fa *FUSEAdapter) Release(path string, fh uint64) (errc int) {
	return 0
}

func (fa *FUSEAdapter) Releasedir(path string, fh uint64) (errc int) {
	return 0
}

func (fa *FUSEAdapter) Statfs(path string, stat *fuse.Statfs_t) (errc int) {
	stat.Bsize = 4096
	stat.Blocks = 128 * 1024 * 1024
	stat.Bfree = 100 * 1024 * 1024
	stat.Bavail = 100 * 1024 * 1024
	stat.Frsize = 4096
	stat.Namemax = 255
	return 0
}

func (fa *FUSEAdapter) Chmod(path string, mode uint32) (errc int) { return 0 }
func (fa *FUSEAdapter) Chown(path string, uid uint32, gid uint32) (errc int) { return 0 }
func (fa *FUSEAdapter) Setxattr(path string, name string, value []byte, flags int) (errc int) {
	return 0
}
func (fa *FUSEAdapter) Getxattr(path string, name string) (int, []byte) {
	return -fuse.ENOATTR, nil
}
func (fa *FUSEAdapter) Removexattr(path string, name string) (errc int) {
	return 0
}
func (fa *FUSEAdapter) Listxattr(path string, fill func(name string) bool) (errc int) {
	return 0
}

// ——— Lifecycle ———

// Shutdown marks the adapter as shutting down and shuts down the underlying VFS.
func (fa *FUSEAdapter) Shutdown() {
	atomic.StoreInt32(&fa.shuttingDown, 1)
	fa.vfs.Shutdown()
}

// InvalidateDirCache forwards to the underlying VFS implementation.
// This is called by the daemon's CacheInvalidator when external uploads complete.
func (fa *FUSEAdapter) InvalidateDirCache(path string) {
	// If the VFS supports cache invalidation, forward the call.
	// VFSImpl implements this; other VFS implementations may ignore it.
	if ci, ok := fa.vfs.(interface{ InvalidateDirCache(string) }); ok {
		ci.InvalidateDirCache(path)
	}
}
