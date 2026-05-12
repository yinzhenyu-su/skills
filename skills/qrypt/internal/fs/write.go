package fs

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/log"
)

func (fs *QryptFS) Create(path string, flags int, mode uint32) (errc int, fh uint64) {
	defer func() {
		if r := recover(); r != nil {
			log.L.Errorf("PANIC in Create(%s): %v\n", path, r)
			errc = -fuse.EIO
		}
	}()
	if fs.IsShuttingDown() {
	log.L.Warnf("[SHUTDOWN] Rejecting Create: %s\n", path)
		return -fuse.EIO, 0
	}
	if strings.Contains(path, "/.DS_Store") || strings.Contains(path, "/._") {
		return -fuse.ENOENT, 0
	}
	log.L.Infof("[FUSE] Create: path=%s, flags=%d, mode=%o\n", path, flags, mode)
	parentPath := filepath.Dir(path)
	parentNode, errc := fs.lookup(parentPath)
	if errc != 0 {
		return errc, 0
	}

	name := filepath.Base(path)
	n := &Node{
		fid:               "local_" + name + "_" + fmt.Sprint(time.Now().UnixNano()),
		parentFid:         parentNode.fid,
		name:              name,
		currentPath:       path,
		isFolder:          false,
		mtime:             time.Now(),
		isDirty:           true,
		baseServerMtime:   0,
		baseServerSize:    0,
		lastMetadataCheck: time.Now(),
		source:            "local",
	}

	nonce, err := fs.cipher.GenerateRandomNonce()
	if err == nil {
		n.fileNonce = nonce
		n.hasNonce = true
	}
	if fs.staging == nil {
		return -fuse.EIO, 0
	}
	localPath, err := fs.staging.Create(n.fid)
	if err != nil {
		return -fuse.EIO, 0
	}
	n.localPath = localPath

	fs.storeNode(path, n)
	fs.cacheSvc.DeleteNeg(parentNode.fid, name)
	n.mu.Lock()
	fs.maybeSavePendingNodeLocked(path, n, true)
	n.mu.Unlock()
	return 0, uint64(uintptr(unsafe.Pointer(n)))
}

func (fs *QryptFS) Mknod(path string, mode uint32, dev uint64) (errc int) {
	err, _ := fs.Create(path, 0, mode)
	return err
}

func (fs *QryptFS) Write(path string, buff []byte, ofst int64, fh uint64) (n int) {
	defer func() {
		if r := recover(); r != nil {
			log.L.Errorf("PANIC in Write(%s): %v\n", path, r)
			n = 0
		}
	}()
	if fs.IsShuttingDown() {
		log.L.Warnf("[SHUTDOWN] Rejecting Write: %s\n", path)
		return 0
	}
	if strings.Contains(path, "/.DS_Store") || strings.Contains(path, "/._") {
		return 0
	}
	log.L.Infof("[FUSE] Write: path=%s, len=%d, offset=%d, fh=%d\n", path, len(buff), ofst, fh)
	node, errc := fs.lookupExtended(path, false)
	if errc != 0 {
		return 0
	}

	node.mu.Lock()
	if fs.staging == nil {
		node.mu.Unlock()
		return 0
	}
	if node.localPath == "" {
		newFid := "local_" + node.name + "_" + fmt.Sprint(time.Now().UnixNano())
		localPath, err := fs.staging.Create(newFid)
		if err != nil {
			node.mu.Unlock()
			return 0
		}
		node.localPath = localPath
	}

	node.isDirty = true
	node.mtime = time.Now()
	written, err := fs.staging.WriteAt(node.localPath, buff, ofst)
	if err != nil {
		node.mu.Unlock()
		return 0
	}
	if ofst+int64(written) > node.size {
		node.size = ofst + int64(written)
	}

	fs.maybeSavePendingNodeLocked(path, node, false)
	node.mu.Unlock()
	return written
}

func (fs *QryptFS) Truncate(path string, size int64, fh uint64) (errc int) {
	defer func() {
		if r := recover(); r != nil {
			errc = -fuse.EIO
		}
	}()
	if fs.IsShuttingDown() {
		return -fuse.EIO
	}
	n, errc := fs.lookup(path)
	if errc != 0 {
		return errc
	}
	if n.isFolder {
		return -fuse.EISDIR
	}
	if size < 0 {
		return -fuse.EINVAL
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	n.size = size
	n.isDirty = true
	n.mtime = time.Now()
	if fs.staging == nil {
		return -fuse.EIO
	}
	if n.localPath == "" {
		newFid := "local_" + n.name + "_" + fmt.Sprint(time.Now().UnixNano())
		localPath, err := fs.staging.Create(newFid)
		if err != nil {
			return -fuse.EIO
		}
		n.localPath = localPath
	}
	if err := fs.staging.Truncate(n.localPath, size); err != nil {
		return -fuse.EIO
	}
	fs.maybeSavePendingNodeLocked(path, n, true)
	return 0
}

func (fs *QryptFS) Flush(path string, fh uint64) (errc int) {
	return 0
}

func (fs *QryptFS) Chmod(path string, mode uint32) (errc int) {
	_, errc = fs.lookup(path)
	return
}

func (fs *QryptFS) Chown(path string, uid uint32, gid uint32) (errc int) {
	_, errc = fs.lookup(path)
	return
}

func (fs *QryptFS) Utimens(path string, tmsp []fuse.Timespec) (errc int) {
	n, errc := fs.lookup(path)
	if errc != 0 {
		return errc
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(tmsp) > 1 {
		n.mtime = tmsp[1].Time()
	} else {
		n.mtime = time.Now()
	}
	return 0
}

func (fs *QryptFS) Setxattr(path string, name string, value []byte, flags int) (errc int) {
	_, errc = fs.lookup(path)
	return
}

func (fs *QryptFS) Getxattr(path string, name string) (int, []byte) {
	_, errc := fs.lookup(path)
	if errc != 0 {
		return errc, nil
	}
	return -fuse.ENOATTR, nil
}

func (fs *QryptFS) Removexattr(path string, name string) (errc int) {
	_, errc = fs.lookup(path)
	return -fuse.ENOATTR
}

func (fs *QryptFS) Listxattr(path string, fill func(name string) bool) (errc int) {
	_, errc = fs.lookup(path)
	return
}

func (fs *QryptFS) Release(path string, fh uint64) (errc int) {
	defer func() {
		if r := recover(); r != nil {
			log.L.Errorf("PANIC in Release(%s): %v\n", path, r)
			errc = -fuse.EIO
		}
	}()
	if fs.IsShuttingDown() {
		log.L.Warnf("[SHUTDOWN] Rejecting Release: %s\n", path)
		return 0
	}
	log.L.Infof("[FUSE] Release: path=%s, fh=%d\n", path, fh)
	node, errc := fs.lookup(path)
	if errc != 0 {
		return errc
	}

	node.mu.RLock()
	dirty := node.isDirty
	node.mu.RUnlock()

	if dirty {
		fs.enqueueSyncDelay(node, 200*time.Millisecond)
	}
	return 0
}

func (fs *QryptFS) Open(path string, flags int) (errc int, fh uint64) {
	n, errc := fs.lookup(path)
	if errc != 0 {
		return errc, 0
	}
	return 0, uint64(uintptr(unsafe.Pointer(n)))
}

func (fs *QryptFS) Fsync(path string, datasync bool, fh uint64) (errc int) {
	log.L.Infof("[FUSE] Fsync: path=%s, datasync=%v, fh=%d\n", path, datasync, fh)
	node, errc := fs.lookup(path)
	if errc != 0 {
		return errc
	}
	node.mu.RLock()
	dirty := node.isDirty
	localPath := node.localPath
	node.mu.RUnlock()
	if dirty && localPath != "" && fs.staging != nil {
		if err := fs.staging.Sync(localPath); err != nil {
			return -fuse.EIO
		}
	}
	return 0
}

func (fs *QryptFS) Ftruncate(path string, size int64, fh uint64) (errc int) {
	log.L.Infof("[FUSE] Ftruncate: path=%s, size=%d, fh=%d\n", path, size, fh)
	return fs.Truncate(path, size, fh)
}

func (fs *QryptFS) Releasedir(path string, fh uint64) (errc int) {
	return 0
}

func (fs *QryptFS) Init() {
	log.L.Infof("[FUSE] Init: QryptFS starting up\n")
}

func (fs *QryptFS) Destroy() {
	log.L.Infof("[FUSE] Destroy: QryptFS shutting down\n")
}
