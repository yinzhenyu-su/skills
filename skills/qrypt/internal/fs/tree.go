package fs

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/log"
	"github.com/yinzhenyu/skills/qrypt/internal/quark"
)

func (fs *QryptFS) storeNode(path string, n *Node) {
	path = filepath.Clean(path)
	n.mu.Lock()
	n.currentPath = path
	if n.isFolder && n.children == nil {
		n.children = make(map[string]*Node)
	}
	n.mu.Unlock()

	log.L.Debugf("storeNode: path=%s fid=%s isFolder=%v\n", path, n.fid, n.isFolder)
	fs.nodes.Store(path, n)

	if path != "/" {
		parentPath := filepath.Dir(path)
		if v, ok := fs.nodes.Load(parentPath); ok {
			p := v.(*Node)
			p.mu.Lock()
			if p.children == nil {
				p.children = make(map[string]*Node)
			}
			if n.fid != "" && n.fid == p.fid {
				p.mu.Unlock()
				return
			}
			p.children[filepath.Base(path)] = n
			p.mtime = time.Now()
			p.mu.Unlock()
		}
	}

	n.mu.RLock()
	fid := n.fid
	n.mu.RUnlock()
	if fid != "" && !strings.HasPrefix(fid, "local_") {
		fs.fidNodes.Store(fid, n)
	}
}

func (fs *QryptFS) replaceNodePath(oldPath, newPath string, n *Node) {
	if oldPath != newPath {
		fs.nodes.Delete(oldPath)
		oldParent := filepath.Dir(oldPath)
		newParent := filepath.Dir(newPath)
		if oldParent != newParent {
			if v, ok := fs.nodes.Load(oldParent); ok {
				p := v.(*Node)
				p.mu.Lock()
				if p.children != nil {
					delete(p.children, filepath.Base(oldPath))
				}
				p.mu.Unlock()
			}
		} else if oldPath != "" {
			if v, ok := fs.nodes.Load(oldParent); ok {
				p := v.(*Node)
				p.mu.Lock()
				if p.children != nil {
					delete(p.children, filepath.Base(oldPath))
					p.children[filepath.Base(newPath)] = n
				}
				p.mu.Unlock()
			}
		}
	}

	n.mu.Lock()
	n.currentPath = newPath
	if n.isFolder && n.children == nil {
		n.children = make(map[string]*Node)
	}
	n.mu.Unlock()

	fs.nodes.Store(newPath, n)

	if oldPath != newPath {
		parentPath := filepath.Dir(newPath)
		if v, ok := fs.nodes.Load(parentPath); ok {
			p := v.(*Node)
			p.mu.Lock()
			if p.children == nil {
				p.children = make(map[string]*Node)
			}
			p.children[filepath.Base(newPath)] = n
			p.mu.Unlock()
		}
	}
}

func (fs *QryptFS) deleteNodePath(path string, n *Node) {
	path = filepath.Clean(path)
	log.L.Debugf("deleteNodePath: path=%s fid=%s isFolder=%v\n", path, n.fid, n.isFolder)
	fs.nodes.Delete(path)

	if path != "/" {
		parentPath := filepath.Dir(path)
		if v, ok := fs.nodes.Load(parentPath); ok {
			p := v.(*Node)
			p.mu.Lock()
			if p.children != nil {
				log.L.Debugf("deleteNodePath: removing child %s from parent %s\n", filepath.Base(path), parentPath)
				delete(p.children, filepath.Base(path))
			}
			p.mtime = time.Now()
			p.mu.Unlock()
		} else {
			log.L.Debugf("deleteNodePath: parent %s not in memory (already removed?)\n", parentPath)
		}
	}

	var fid, localPath string
	if n != nil {
		n.mu.RLock()
		fid = n.fid
		localPath = n.localPath
		n.mu.RUnlock()

		if n.currentPath == path {
			n.currentPath = ""
			n.Cancel()
			fs.retryState.Delete(n)
		}

		if localPath != "" && fs.staging != nil {
			log.L.Debugf("deleteNodePath: removing staging %s\n", localPath)
			fs.staging.Remove(localPath)
		}

	}

	if fid != "" && !strings.HasPrefix(fid, "local_") {
		fs.fidNodes.Delete(fid)
	}
	log.L.Debugf("deleteNodePath: done path=%s\n", path)
}

func (fs *QryptFS) deleteSubtreePaths(parentPath string, n *Node) {
	if n == nil || !n.isFolder {
		return
	}

	log.L.Debugf("deleteSubtreePaths: start path=%s\n", parentPath)

	prefix := parentPath
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	n.mu.RLock()
	type entry struct {
		name  string
		child *Node
	}
	var children []entry
	for name, child := range n.children {
		children = append(children, entry{name, child})
	}
	childCount := len(children)
	n.mu.RUnlock()

	log.L.Debugf("deleteSubtreePaths: %d children to clean up under %s\n", childCount, parentPath)

	var fidsToPurge []string

	for _, c := range children {
		childPath := prefix + c.name
		log.L.Debugf("deleteSubtreePaths: processing child %s\n", childPath)

		if v, ok := fs.nodes.Load(childPath); ok && v.(*Node) == c.child {
			fs.nodes.Delete(childPath)
		}

		if c.child != nil {
			c.child.mu.RLock()
			fid := c.child.fid
			isFolder := c.child.isFolder
			localPath := c.child.localPath
			c.child.mu.RUnlock()

			if fid != "" && !strings.HasPrefix(fid, "local_") {
				if v, ok := fs.fidNodes.Load(fid); ok && v.(*Node) == c.child {
					fs.fidNodes.Delete(fid)
				}
				fidsToPurge = append(fidsToPurge, fid)
				if fs.cacheMgr != nil {
					fs.cacheMgr.RemoveChunksByFid(fid)
				}
			}

			if localPath != "" && fs.staging != nil {
				fs.staging.Remove(localPath)
			}

			if isFolder {
				fs.deleteSubtreePaths(childPath, c.child)
			}
		}
	}

	if fs.cacheMgr != nil && len(fidsToPurge) > 0 {
		log.L.Debugf("deleteSubtreePaths: purging %d fids\n", len(fidsToPurge))
		fs.cacheMgr.BatchDeleteNodeState(fidsToPurge, nil)
	}
	log.L.Debugf("deleteSubtreePaths: done path=%s\n", parentPath)
}

func (fs *QryptFS) renameSubtreePaths(oldPath, newPath string) {
	if v, ok := fs.nodes.Load(oldPath); ok {
		n := v.(*Node)
		fs.recursiveRename(oldPath, newPath, n)
	}
}

func (fs *QryptFS) recursiveRename(oldPath, newPath string, n *Node) {
	fs.replaceNodePath(oldPath, newPath, n)

	if !n.isFolder {
		return
	}

	prefixOld := oldPath
	if !strings.HasSuffix(prefixOld, "/") {
		prefixOld += "/"
	}
	prefixNew := newPath
	if !strings.HasSuffix(prefixNew, "/") {
		prefixNew += "/"
	}

	n.mu.RLock()
	type entry struct {
		name  string
		child *Node
	}
	var children []entry
	for name, child := range n.children {
		children = append(children, entry{name, child})
	}
	n.mu.RUnlock()

	for _, c := range children {
		fs.recursiveRename(prefixOld+c.name, prefixNew+c.name, c.child)
	}
}

func (fs *QryptFS) lookup(path string) (*Node, int) {
	return fs.lookupExtended(path, true)
}

func (fs *QryptFS) lookupExtended(path string, refresh bool) (*Node, int) {
	path = filepath.Clean(path)

	if fs.isUnderDeletingDir(path) {
		return nil, -fuse.ENOENT
	}

	if v, ok := fs.nodes.Load(path); ok {
		n := v.(*Node)
		n.mu.RLock()
		isFolder := n.isFolder
		lastCheck := n.lastMetadataCheck
		isDirty := n.isDirty
		fid := n.fid
		parentFid := n.parentFid
		n.mu.RUnlock()

		if !strings.HasPrefix(fid, "local_") {
			if refresh && !isFolder && !isDirty && time.Since(lastCheck) > MetadataTTL {
				if _, inDeletion := fs.activeDeletions.Load(fid); inDeletion {
					fs.deleteNodePath(path, n)
					return nil, -fuse.ENOENT
				}

				files, err := fs.fetchFiles(parentFid)
				found := false
				if err == nil {
					for _, f := range files {
						if f.Fid == fid {
							if _, inDeletion := fs.activeDeletions.Load(fid); inDeletion {
								break
							}
							decSize, err := fs.cipher.DecryptedSize(f.Int64Size())
							if err != nil {
								log.L.Warnf("lookupExtended: DecryptedSize failed for %s fid=%s encSize=%d: %v\n", path, fid, f.Int64Size(), err)
							}
							n.mu.Lock()
							n.size = decSize
							n.encSize = f.Int64Size()
							n.mtime = f.ModTime()
							n.baseServerMtime = f.ModTime().UnixMilli()
							n.baseServerSize = decSize
							n.lastMetadataCheck = time.Now()
							n.mu.Unlock()
							found = true
							break
						}
					}
				}

				if !found && path != "/" {
					if n.source == "local" || n.source == "merged" {
						return n, 0
					}
					fs.deleteNodePath(path, n)
					return nil, -fuse.ENOENT
				}
			}
			return n, 0
		}
		return n, 0
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	currentFid := fs.rootFid
	currentPath := ""

	for _, part := range parts {
		if part == "" {
			continue
		}
		currentPath += "/" + part

		if v, ok := fs.nodes.Load(currentPath); ok {
			currentFid = v.(*Node).fid
			continue
		}

		if fs.isUnderDeletingDir(currentPath) {
			return nil, -fuse.ENOENT
		}

		if _, inDeletion := fs.activeDeletions.Load(currentFid); inDeletion {
			return nil, -fuse.ENOENT
		}

		files, err := fs.fetchFiles(currentFid)
		if err != nil {
			return nil, -fuse.EIO
		}

		found := false
		for _, f := range files {
			decName, _ := fs.cipher.DecryptSegment(f.FileName)
			if decName != part {
				continue
			}
			if _, inDeletion := fs.activeDeletions.Load(f.Fid); inDeletion {
				found = false
				break
			}

			if v, ok := fs.nodes.Load(currentPath); ok {
				existing := v.(*Node)
				existing.mu.RLock()
				dirty := existing.isDirty
				existing.mu.RUnlock()
				if dirty {
					currentFid = existing.fid
					found = true
					break
				}
			}

			decSize, errDec := fs.cipher.DecryptedSize(f.Int64Size())
			if errDec != nil {
				log.L.Warnf("lookupExtended path resolution: DecryptedSize failed for fid=%s name=%s encSize=%d: %v\n", f.Fid, part, f.Int64Size(), errDec)
			}
			modTime := f.ModTime()
			lastCheck := time.Time{}
			if !f.IsDir() {
				lastCheck = time.Now()
			}
			newNode := &Node{
				fid:               f.Fid,
				parentFid:         currentFid,
				name:              decName,
				size:              decSize,
				encSize:           f.Int64Size(),
				currentPath:       currentPath,
				isFolder:          f.IsDir(),
				mtime:             modTime,
				baseServerMtime:   modTime.UnixMilli(),
				baseServerSize:    decSize,
				lastMetadataCheck: lastCheck,
				source:            "remote",
			}
			fs.storeNode(currentPath, newNode)
			currentFid = f.Fid
			found = true
			break
		}

		if !found {
			return nil, -fuse.ENOENT
		}
	}

	if v, ok := fs.nodes.Load(path); ok {
		return v.(*Node), 0
	}
	return nil, -fuse.ENOENT
}

func (fs *QryptFS) isUnderDeletingDir(path string) bool {
	p := path
	for p != "" && p != "/" {
		if _, ok := fs.deletingPaths.Load(p); ok {
			return true
		}
		p = filepath.Dir(p)
	}
	return false
}

func (fs *QryptFS) currentPathForNode(n *Node) string {
	n.mu.RLock()
	path := n.currentPath
	parentFid := n.parentFid
	name := n.name
	n.mu.RUnlock()

	if path != "" {
		return path
	}

	constructedPath := ""
	if parentFid == "" || parentFid == fs.rootFid {
		if name == "" {
			constructedPath = "/"
		} else {
			constructedPath = "/" + name
		}
	} else {
		var parentNode *Node
		if v, ok := fs.fidNodes.Load(parentFid); ok {
			parentNode = v.(*Node)
		}
		if parentNode != nil {
			parentPath := fs.currentPathForNode(parentNode)
			if parentPath != "" {
				if !strings.HasSuffix(parentPath, "/") {
					parentPath += "/"
				}
				constructedPath = parentPath + name
			}
		}
	}

	if constructedPath != "" {
		if v, ok := fs.nodes.Load(constructedPath); ok && v.(*Node) == n {
			n.mu.Lock()
			n.currentPath = constructedPath
			n.mu.Unlock()
			return constructedPath
		}
	}
	return ""
}

func (fs *QryptFS) persistPendingPath(oldPath, newPath string, n *Node) {
	if fs.cacheMgr == nil || n == nil {
		return
	}

	n.mu.RLock()
	shouldSave := n.isDirty && !n.isFolder && n.localPath != ""
	fid := n.fid
	parentFid := n.parentFid
	name := n.name
	localPath := n.localPath
	size := n.size
	nonce := append([]byte(nil), n.fileNonce[:]...)
	baseMtime := n.baseServerMtime
	baseSize := n.baseServerSize
	n.mu.RUnlock()

	if !shouldSave {
		return
	}

	fs.cacheMgr.SavePendingNode(newPath, fid, parentFid, name, localPath, size, false, nonce, baseMtime, baseSize, "", 0)
	if oldPath != "" && oldPath != newPath {
		fs.cacheMgr.RemovePendingNode(oldPath)
	}
}

type fetchFilesResult struct {
	files []quark.File
	err   error
	done  chan struct{}
}

func (fs *QryptFS) fetchFiles(fid string) ([]quark.File, error) {
	if fid == "" {
		return nil, nil
	}
	start := time.Now()
	log.L.Debugf("fetchFiles: fid=%s\n", fid)

	if v, ok := fs.fetchingFiles.Load(fid); ok {
		r := v.(*fetchFilesResult)
		<-r.done
		log.L.Debugf("fetchFiles: waited for concurrent fetch of %s (took %v)\n", fid, time.Since(start))
		return r.files, r.err
	}

	r := &fetchFilesResult{done: make(chan struct{})}
	if actual, loaded := fs.fetchingFiles.LoadOrStore(fid, r); loaded {
		existing := actual.(*fetchFilesResult)
		<-existing.done
		log.L.Debugf("fetchFiles: waited for concurrent fetch of %s (took %v)\n", fid, time.Since(start))
		return existing.files, existing.err
	}
	defer fs.fetchingFiles.Delete(fid)

	r.files, r.err = fs.fileSvc.ListFiles(fid)
	close(r.done)
	log.L.Debugf("fetchFiles: done fid=%s got %d files err=%v (took %v)\n", fid, len(r.files), r.err, time.Since(start))
	return r.files, r.err
}

func (fs *QryptFS) maybeSavePendingNodeLocked(path string, n *Node, force bool) error {
	if fs.cacheMgr == nil {
		return nil
	}
	now := time.Now()
	if !force && now.Sub(n.lastPendingSave) < pendingNodeSaveInterval &&
		(n.size-n.lastPendingSize) < pendingNodeSaveSizeStep {
		return nil
	}
	nonce := n.fileNonce[:]
	err := fs.cacheMgr.SavePendingNode(path, n.fid, n.parentFid, n.name, n.localPath, n.size, n.isFolder, nonce, n.baseServerMtime, n.baseServerSize, "", 0)
	if err == nil {
		n.lastPendingSave = now
		n.lastPendingSize = n.size
	}
	return err
}
