//go:build !nofuse

package fusefs

import (
	"strings"
	"sync"
	"time"

	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
	"github.com/yinzhenyu/skills/qrypt/drivers"
)

func (fs *QryptFS) Readdir(path string, fill func(name string, stat *fuse.Stat_t, ofst int64) bool, ofst int64, fh uint64) (errc int) {
	fill(".", nil, 0)
	fill("..", nil, 0)

	n, errc := fs.lookup(path)
	if errc != 0 {
		return errc
	}

	n.mu.RLock()
	lastCheck := n.lastMetadataCheck
	childCount := len(n.children)
	parentFid := n.fid
	n.mu.RUnlock()

	forceRefresh := childCount == 0 && time.Since(lastCheck) > 5*time.Second

	if time.Since(lastCheck) > MetadataTTL || forceRefresh {
		files, err := fs.fetchFiles(parentFid)
		if err != nil {
			logging.L.Warnf("Readdir: fetchFiles failed for %s: %v, using cached children\n", path, err)
		} else {
			fs.MergeRemoteChanges(path, parentFid, files)

			n.mu.Lock()
			n.lastMetadataCheck = time.Now()
			n.mu.Unlock()

			n.mu.RLock()
			var childrenToPrefetch []*Node
			for _, child := range n.children {
				childrenToPrefetch = append(childrenToPrefetch, child)
			}
			n.mu.RUnlock()

			for _, child := range childrenToPrefetch {
				child.mu.RLock()
				isDir := child.isFolder
				childFid := child.fid
				childPath := child.currentPath
				childLastCheck := child.lastMetadataCheck
				child.mu.RUnlock()

				if isDir && childFid != "" && !strings.HasPrefix(childFid, "local_") && time.Since(childLastCheck) > MetadataTTL {
					if _, inDeletion := fs.activeDeletions.Load(childFid); inDeletion {
						continue
					}
					if fs.isUnderDeletingDir(childPath) {
						continue
					}

					go func(fid, cpath string) {
						select {
						case fs.prefetchSem <- struct{}{}:
							defer func() { <-fs.prefetchSem }()
						default:
							return
						}
						prefetchFiles, err := fs.fetchFiles(fid)
						if err != nil {
							return
						}
						if fs.isUnderDeletingDir(cpath) {
							return
						}
						fs.MergeRemoteChanges(cpath, fid, prefetchFiles)
						if cpn, ok := fs.fidNodes.Load(fid); ok {
							cn := cpn.(*Node)
							cn.mu.Lock()
							cn.lastMetadataCheck = time.Now()
							cn.mu.Unlock()
						}
					}(childFid, childPath)
				}
			}
		}
	}

	uid, gid, _ := fuse.Getcontext()

	n.mu.RLock()
	defer n.mu.RUnlock()

	for name, child := range n.children {
		child.mu.RLock()
		isFolder := child.isFolder
		size := child.size
		mtime := child.mtime
		child.mu.RUnlock()

		stat := &fuse.Stat_t{}
		stat.Uid = uid
		stat.Gid = gid
		if isFolder {
			stat.Mode = fuse.S_IFDIR | 0755
		} else {
			stat.Mode = fuse.S_IFREG | 0644
			stat.Size = size
		}
		stat.Mtim = fuse.NewTimespec(mtime)
		stat.Atim = stat.Mtim
		stat.Ctim = stat.Mtim

		fill(name, stat, 0)
	}

	return 0
}

func (fs *QryptFS) MergeRemoteChanges(parentPath string, parentFid string, remoteFiles []drivers.Entry) {
	if parentFid == "" {
		return
	}

	waitChan, loading := fs.merging.LoadOrStore(parentFid, make(chan struct{}))
	if loading {
		<-waitChan.(chan struct{})
		return
	}
	defer func() {
		close(waitChan.(chan struct{}))
		fs.merging.Delete(parentFid)
	}()

	if fs.isUnderDeletingDir(parentPath) {
		return
	}
	if _, inDeletion := fs.activeDeletions.Load(parentFid); inDeletion {
		return
	}

	seenFids := make(map[string]bool)
	remoteMap := make(map[string]drivers.Entry)
	remoteFids := make(map[string]bool)

	for _, f := range remoteFiles {
		if f.ID == parentFid {
			continue
		}
		seenFids[f.ID] = true
		remoteFids[f.ID] = true

		decName := ""
		if v, ok := fs.fidNodes.Load(f.ID); ok {
			pn := v.(*Node)
			pn.mu.RLock()
			decName = pn.name
			pn.mu.RUnlock()
		}
		if decName == "" {
			decName, _ = fs.cipher.DecryptSegment(f.Name)
		}

		if decName == "" || decName == "." || decName == ".." {
			continue
		}

		if _, exists := remoteMap[decName]; exists {
			continue
		}
		remoteMap[decName] = f
	}

	prefix := parentPath
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	type nodeEntry struct {
		path string
		node *Node
	}
	var localEntries []nodeEntry

	if v, ok := fs.nodes.Load(parentPath); ok {
		p := v.(*Node)
		p.mu.RLock()
		for name, child := range p.children {
			localEntries = append(localEntries, nodeEntry{path: prefix + name, node: child})
		}
		p.mu.RUnlock()
	}

	seenLocalNames := make(map[string]bool)
	syncInProgressCount := 0
	remoteDeletedCount := 0

	for _, entry := range localEntries {
		n := entry.node
		n.mu.RLock()
		name := n.name
		fid := n.fid
		isDirty := n.isDirty
		baseMtime := n.baseServerMtime
		syncQueued := n.syncQueued
		lastUpload := n.lastUploadTime
		expectedFid := n.expectedFid
		n.mu.RUnlock()

		seenLocalNames[name] = true

		if syncQueued {
			syncInProgressCount++
			continue
		}

		rf, exists := remoteMap[name]

		if exists && rf.ID == expectedFid {
			n.mu.Lock()
			if n.fid != expectedFid {
				n.fid = expectedFid
			}
			n.source = "remote"
			n.mu.Unlock()
		}

		if !isDirty && (n.source == "local" || n.source == "merged" || (!lastUpload.IsZero() && time.Since(lastUpload) < 30*time.Second)) {
			if !exists {
				continue
			}
			if rf.ID == fid {
				n.mu.Lock()
				if n.source != "remote" {
					n.source = "remote"
				}
				n.mu.Unlock()
			}
		}

		if !exists {
			if isDirty {
				continue
			}
			if n.source == "local" || n.source == "merged" {
				continue
			}
			if strings.HasPrefix(fid, "local_") {
				continue
			}
			fs.deleteNodePath(entry.path, n)
			remoteDeletedCount++
			continue
		}

		if !strings.HasPrefix(fid, "local_") {
			if rf.ID != fid {
				if !isDirty {
					// Skip fid update if we recently uploaded — Quark API
					// index may not have caught up yet (stale listing).
					if !lastUpload.IsZero() && time.Since(lastUpload) < 30*time.Second {
						// index delay — keep our current fid
					} else {
						n.mu.Lock()
						n.fid = rf.ID
						n.source = "remote"
						n.mu.Unlock()
					}
				}
			}

			remoteMtime := rf.ModTime.UnixMilli()
			n.mu.RLock()
			source := n.source
			n.mu.RUnlock()

			if source == "remote" && baseMtime > 0 && remoteMtime > baseMtime+2000 {
				if !isDirty {
					decSize, errDec := fs.cipher.DecryptedSize(rf.Size)
					if errDec != nil {
						logging.L.Warnf("MergeRemoteChanges: DecryptedSize failed for %s fid=%s encSize=%d: %v\n", entry.path, rf.ID, rf.Size, errDec)
					}
					n.mu.Lock()
					n.fid = rf.ID
					n.size = decSize
					n.encSize = rf.Size
					n.mtime = rf.ModTime
					n.baseServerMtime = remoteMtime
					n.baseServerSize = decSize
					n.lastMetadataCheck = time.Now()
					n.source = "remote"
					n.mu.Unlock()
					if fs.cacheMgr != nil {
						fs.cacheMgr.RemoveChunksByFid(fid)
					}
				} else {
					continue
				}
			} else if isDirty {
				continue
			}
		} else {
			continue
		}
	}

	addedCount := 0
	for name, rf := range remoteMap {
		if seenLocalNames[name] {
			continue
		}
		if _, inDeletion := fs.activeDeletions.Load(rf.ID); inDeletion {
			logging.L.Debugf("MergeRemoteChanges: skipping resurrected file %s (fid=%s) in %s\n", name, rf.ID, parentPath)
			continue
		}
		logging.L.Debugf("MergeRemoteChanges: adding new remote file %s (fid=%s, size=%d) to %s\n", name, rf.ID, rf.Size, parentPath)

		childPath := prefix + name
		decSize, errDec := fs.cipher.DecryptedSize(rf.Size)
		if errDec != nil {
			logging.L.Warnf("MergeRemoteChanges new file: DecryptedSize failed for %s fid=%s encSize=%d: %v\n", childPath, rf.ID, rf.Size, errDec)
		}
		modTime := rf.ModTime
		lastCheck := time.Now()
		if rf.IsDir {
			lastCheck = time.Time{}
		}
		fs.storeNode(childPath, &Node{
			fid:               rf.ID,
			parentFid:         parentFid,
			name:              name,
			size:              decSize,
			encSize:           rf.Size,
			currentPath:       childPath,
			isFolder:          rf.IsDir,
			mtime:             modTime,
			baseServerMtime:   modTime.UnixMilli(),
			baseServerSize:    decSize,
			lastMetadataCheck: lastCheck,
			source:            "remote",
		})
		addedCount++
	}

	if val, ok := fs.deletionsByParent.Load(parentFid); ok {
		childMap := val.(*sync.Map)
		childMap.Range(func(key, value interface{}) bool {
			fid := key.(string)
			if stateVal, exists := fs.activeDeletions.Load(fid); exists {
				state := stateVal.(*deletionState)
				if state.apiDone {
					if !remoteFids[fid] {
						if state.path != "" {
							fs.deletingPaths.Delete(state.path)
						}
						fs.activeDeletions.Delete(fid)
						childMap.Delete(fid)
						fs.purgeTombstonesRecursively(fid)
					}
				}
			} else {
				childMap.Delete(fid)
			}
			return true
		})
	}
}

func (fs *QryptFS) purgeTombstonesRecursively(parentFid string) {
	if parentFid == "" {
		return
	}
	if val, ok := fs.deletionsByParent.Load(parentFid); ok {
		childMap := val.(*sync.Map)
		childMap.Range(func(key, value interface{}) bool {
			fid := key.(string)
			if fid == parentFid {
				childMap.Delete(fid)
				return true
			}
			if stateVal, ok := fs.activeDeletions.Load(fid); ok {
				state := stateVal.(*deletionState)
				if state.path != "" {
					fs.deletingPaths.Delete(state.path)
				}
			}
			fs.activeDeletions.Delete(fid)
			fs.purgeTombstonesRecursively(fid)
			return true
		})
		fs.deletionsByParent.Delete(parentFid)
	}
}
