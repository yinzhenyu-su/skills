//go:build !nofuse

package fusefs

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
)

func (fs *QryptFS) Unlink(path string) (errc int) {
	defer func() {
		if r := recover(); r != nil {
			logging.L.Errorf("PANIC in Unlink(%s): %v\n", path, r)
			errc = -fuse.EIO
		}
	}()
	if fs.IsShuttingDown() {
		logging.L.Warnf("[SHUTDOWN] Rejecting Unlink: %s\n", path)
		return -fuse.EIO
	}
	logging.L.Infof("[FUSE] Unlink: path=%s\n", path)

	n, errc := fs.lookupExtended(path, false)
	if errc != 0 {
		return errc
	}
	if n.isFolder {
		return -fuse.EISDIR
	}

	n.mu.Lock()
	fid := n.fid
	parentFid := n.parentFid
	n.mu.Unlock()

	if !strings.HasPrefix(fid, "local_") {
		if _, exists := fs.activeDeletions.Load(fid); exists {
			fs.deleteNodePath(path, n)
			return 0
		}

		fs.activeDeletions.Store(fid, &deletionState{
			parentFid: parentFid,
			path:      path,
			apiDone:   false,
		})
		actualMap, _ := fs.deletionsByParent.LoadOrStore(parentFid, &sync.Map{})
		actualMap.(*sync.Map).Store(fid, true)

		fs.metadataOpChan <- metadataTask{
			opType: "DELETE",
			path:   path,
			node:   n,
			fids:   []string{fid},
		}
	} else {
		fs.metadataOpChan <- metadataTask{
			opType: "LOCAL_CLEANUP",
			path:   path,
			node:   n,
		}
	}

	fs.deleteNodePath(path, n)
	return 0
}

func (fs *QryptFS) rmdirEmpty(path string, n *Node, parentFid string) int {
	if _, exists := fs.activeDeletions.Load(n.fid); exists {
		fs.deleteNodePath(path, n)
		return 0
	}

	fs.activeDeletions.Store(n.fid, &deletionState{
		parentFid: parentFid,
		path:      path,
		apiDone:   false,
	})
	actualMap, _ := fs.deletionsByParent.LoadOrStore(parentFid, &sync.Map{})
	actualMap.(*sync.Map).Store(n.fid, true)

	fs.metadataOpChan <- metadataTask{
		opType: "DELETE",
		path:   path,
		node:   n,
		fids:   []string{n.fid},
	}

	fs.deleteNodePath(path, n)
	return 0
}

func (fs *QryptFS) rmdirNonEmpty(path string, n *Node, parentFid string) int {
	logging.L.Infof("Rmdir: %s is NOT empty, deleting children recursively\n", path)

	realFids := fs.rmdirDeleteFids(n)

	for _, fid := range realFids {
		if _, exists := fs.activeDeletions.Load(fid); exists {
			continue
		}
		fs.activeDeletions.Store(fid, &deletionState{
			parentFid: parentFid,
			apiDone:   false,
		})
	}

	if parentFid != "" {
		actualMap, _ := fs.deletionsByParent.LoadOrStore(parentFid, &sync.Map{})
		for _, fid := range realFids {
			actualMap.(*sync.Map).Store(fid, true)
		}
	}

	if len(realFids) > 0 {
		fs.metadataOpChan <- metadataTask{
			opType: "DELETE",
			path:   path,
			node:   n,
			fids:   realFids,
		}
	}

	fs.cleanupLocalUploadState(path, n, true)
	fs.deleteSubtreePaths(path, n)
	fs.deleteNodePath(path, n)
	logging.L.Infof("Rmdir: queued recursive delete for %s\n", path)
	return 0
}

func (fs *QryptFS) rmdirLocalNonEmpty(path string, n *Node) int {
	fs.metadataOpChan <- metadataTask{
		opType: "LOCAL_CLEANUP_DIR",
		path:   path,
		node:   n,
	}

	fs.cleanupLocalUploadState(path, n, true)
	fs.deleteSubtreePaths(path, n)
	fs.deleteNodePath(path, n)
	return 0
}

func (fs *QryptFS) Rmdir(path string) (errc int) {
	defer func() {
		if r := recover(); r != nil {
			logging.L.Errorf("PANIC in Rmdir(%s): %v\n", path, r)
			errc = -fuse.EIO
		}
	}()
	if fs.IsShuttingDown() {
		logging.L.Warnf("[SHUTDOWN] Rejecting Rmdir: %s\n", path)
		return -fuse.EIO
	}
	logging.L.Infof("[FUSE] Rmdir: path=%s\n", path)

	n, errc := fs.lookupExtended(path, false)
	if errc != 0 {
		return errc
	}
	if !n.isFolder {
		return -fuse.ENOTDIR
	}

	if atomic.LoadInt32(&n.uploadingChildren) > 0 {
		return -fuse.ENOTEMPTY
	}

	n.mu.Lock()
	parentFid := n.parentFid
	hasChildren := len(n.children) > 0
	n.mu.Unlock()

	fs.deletingPaths.Store(path, struct{}{})

	if hasChildren && strings.HasPrefix(n.fid, "local_") {
		fs.deletingPaths.Delete(path)
		return fs.rmdirLocalNonEmpty(path, n)
	}

	if hasChildren {
		return fs.rmdirNonEmpty(path, n, parentFid)
	}

	return fs.rmdirEmpty(path, n, parentFid)
}

func (fs *QryptFS) rmdirDeleteFids(n *Node) []string {
	if n == nil {
		return nil
	}
	if _, ok := fs.drv.(interface {
		BatchRemove(context.Context, []drivers.Entry) error
	}); ok {
		n.mu.RLock()
		fid := n.fid
		n.mu.RUnlock()
		if fid != "" && !strings.HasPrefix(fid, "local_") {
			return []string{fid}
		}
		return nil
	}

	var fids []string
	collectRemoteFidsPostorder(n, &fids)
	return fids
}

func collectRemoteFidsPostorder(n *Node, fids *[]string) {
	if n == nil {
		return
	}
	n.mu.RLock()
	fid := n.fid
	isFolder := n.isFolder
	var children []*Node
	if isFolder {
		for _, child := range n.children {
			children = append(children, child)
		}
	}
	n.mu.RUnlock()

	for _, child := range children {
		collectRemoteFidsPostorder(child, fids)
	}
	if fid != "" && !strings.HasPrefix(fid, "local_") {
		*fids = append(*fids, fid)
	}
}
