//go:build !nofuse

package fusefs

import (
	"github.com/winfsp/cgofuse/fuse"
)

func (fs *QryptFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	n, errc := fs.lookupExtended(path, false)
	if errc != 0 {
		return errc
	}

	uid, gid, _ := fuse.Getcontext()
	stat.Uid = uid
	stat.Gid = gid

	if n.isFolder {
		stat.Mode = fuse.S_IFDIR | 0755
		stat.Nlink = 2
		n.mu.RLock()
		stat.Size = int64(len(n.children))
		n.mu.RUnlock()
	} else {
		stat.Mode = fuse.S_IFREG | 0644
		stat.Size = n.size
		stat.Nlink = 1
	}
	stat.Mtim = fuse.NewTimespec(n.mtime)
	stat.Atim = stat.Mtim
	stat.Ctim = stat.Mtim
	return 0
}

func (fs *QryptFS) Access(path string, mask uint32) (errc int) {
	_, errc = fs.lookup(path)
	return
}
