//go:build !nofuse

package fusefs

import (
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
)

const (
	RENAME_NOREPLACE = 1
)

func (fs *QryptFS) Rename(oldPath string, newPath string) (errc int) {
	return fs.Rename3(oldPath, newPath, 0)
}

func (fs *QryptFS) Rename3(oldPath string, newPath string, flags uint32) (errc int) {
	defer func() {
		if r := recover(); r != nil {
			logging.L.Errorf("PANIC in Rename(%s->%s): %v\n", oldPath, newPath, r)
			errc = -fuse.EIO
		}
	}()
	if fs.IsShuttingDown() {
		logging.L.Warnf("[SHUTDOWN] Rejecting Rename: %s -> %s\n", oldPath, newPath)
		return -fuse.EIO
	}
	logging.L.Infof("[FUSE] Rename: oldPath=%s, newPath=%s\n", oldPath, newPath)

	oldNode, errc := fs.lookup(oldPath)
	if errc != 0 {
		return errc
	}

	if oldNode.isFolder && atomic.LoadInt32(&oldNode.uploadingChildren) > 0 {
		return -fuse.EBUSY
	}

	if flags&RENAME_NOREPLACE != 0 {
		if _, errc := fs.lookupExtended(newPath, false); errc == 0 {
			logging.L.Warnf("Rename: target exists (NOREPLACE) %s -> %s\n", oldPath, newPath)
			return -fuse.EEXIST
		}
	}

	oldParent := filepath.Dir(oldPath)
	newParent := filepath.Dir(newPath)
	newName := filepath.Base(newPath)

	// Submit rename to coalescer (replaces synchronous driver.Rename/Move calls).
	// The coalescer will execute the rename asynchronously after the batching window.
	fs.coalescer.Submit(FileOp{
		Kind:      OpRename,
		Path:      oldPath,
		NewPath:   newPath,
		Fid:       oldNode.fid,
		ParentFid: oldNode.parentFid,
		IsDir:     oldNode.isFolder,
		Time:      time.Now(),
		Node:      oldNode,
	})

	if oldParent != newParent {
		newParentNode, errc := fs.lookup(newParent)
		if errc != 0 {
			return errc
		}
		oldNode.mu.Lock()
		oldNode.parentFid = newParentNode.fid
		oldNode.mu.Unlock()
	}

	oldNode.mu.Lock()
	oldNode.name = newName
	oldNode.mu.Unlock()

	fs.recursiveRename(oldPath, newPath, oldNode)
	fs.persistPendingPath(oldPath, newPath, oldNode)

	return 0
}
