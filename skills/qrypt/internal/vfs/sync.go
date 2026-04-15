package vfs

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/driver"
	uploadpkg "github.com/yinzhenyu/skills/qrypt/internal/upload"
)

func (fs *QryptFS) recoverDirtyFiles() {
	if fs.cache == nil {
		return
	}
	pending, err := fs.cache.GetPendingNodes()
	if err != nil {
		driver.Log.Printf("Failed to recover dirty files: %v\n", err)
		return
	}

	for _, p := range pending {
		if p.Path == "" || !strings.HasPrefix(p.Path, "/") || p.Fid == "" || p.Name == "" || p.IsFolder {
			driver.Log.Printf("Drop invalid pending entry: path=%q fid=%q\n", p.Path, p.Fid)
			fs.cleanupPendingEntry(p)
			continue
		}
		if p.LocalPath == "" || fs.staging == nil || !fs.staging.Exists(p.LocalPath) {
			driver.Log.Printf("Drop pending entry with missing staging file: path=%q localPath=%q\n", p.Path, p.LocalPath)
			fs.cleanupPendingEntry(p)
			continue
		}

		// If fid doesn't start with "local_", file was previously synced.
		// Verify it still exists on server before re-uploading.
		if !strings.HasPrefix(p.Fid, "local_") {
			if fs.fileExistsOnServer(p.Fid, p.ParentFid) {
				driver.Log.Printf("Skip recovered pending upload for already-synced file that still exists: path=%q fid=%q\n", p.Path, p.Fid)
				fs.cleanupPendingEntry(p)
				continue
			}
			driver.Log.Printf("File previously synced but no longer exists on server: path=%q fid=%q, will re-upload\n", p.Path, p.Fid)
		}

		n := &node{
			fid:               p.Fid,
			parentFid:         p.ParentFid,
			name:              p.Name,
			localPath:         p.LocalPath,
			currentPath:       p.Path,
			size:              p.Size,
			isFolder:          p.IsFolder,
			mtime:             time.Now(),
			isDirty:           true,
			syncQueued:        true,
			baseServerMtime:   p.BaseServerMtime,
			baseServerSize:    p.BaseServerSize,
			lastMetadataCheck: time.Now(),
		}
		if len(p.Nonce) == 24 {
			copy(n.fileNonce[:], p.Nonce)
			n.hasNonce = true
		}
		fs.storeNode(p.Path, n)
		fs.uploadChan <- syncTask{node: n}
		driver.Log.Printf("Recovered pending upload: %s\n", p.Path)
	}
}

func (fs *QryptFS) cleanupPendingEntry(p cache.CacheDBPendingNode) {
	_ = fs.cache.RemovePendingNode(p.Path)
	if p.Fid != "" {
		_ = fs.cache.RemovePendingNodesByFid(p.Fid)
		_ = fs.cache.RemoveChunksByFid(p.Fid)
		_ = fs.cache.RemoveStagingMeta(p.Fid)
	}
}

// fileExistsOnServerDetailed checks if a file with the given fid exists and returns its metadata.
func (fs *QryptFS) fileExistsOnServerDetailed(fid, parentFid string) (*driver.File, error) {
	if parentFid == "" || parentFid == "root" || isFinderTrashPath("/"+filepath.ToSlash(parentFid)) {
		return nil, nil
	}
	fs.driver.RemoveDirCache(parentFid)
	files, err := fs.driver.ListFiles(parentFid)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if f.Fid == fid {
			return &f, nil
		}
	}
	return nil, nil
}

// fileExistsOnServer checks if a file with the given fid exists in the parent directory on the server.
func (fs *QryptFS) fileExistsOnServer(fid, parentFid string) bool {
	f, _ := fs.fileExistsOnServerDetailed(fid, parentFid)
	return f != nil
}

func (fs *QryptFS) uploadWorker() {
	for task := range fs.uploadChan {
		func(t syncTask) {
			n := t.node

			if _, loaded := fs.syncing.LoadOrStore(n, struct{}{}); loaded {
				return
			}
			defer fs.syncing.Delete(n)

			path := fs.currentPathForNode(n)
			if path == "" {
				n.mu.Lock()
				n.syncQueued = false
				n.mu.Unlock()
				return
			}

			n.mu.RLock()
			dirty := n.isDirty
			n.mu.RUnlock()
			if !dirty {
				n.mu.Lock()
				n.syncQueued = false
				n.mu.Unlock()
				return
			}

			driver.Log.Printf("Background Sync Start: %s (fid=%s)\n", path, n.fid)
			fs.notifySyncStart(path, n)
			err := fs.syncFile(path, n)

			n.mu.Lock()
			n.syncQueued = false
			stillDirty := n.isDirty
			n.mu.Unlock()

			if err != nil {
				if strings.Contains(err.Error(), errNonRetryableSync.Error()) {
					driver.Log.Printf("Background Sync Non-Retryable for %s (fid=%s): %v\n", path, n.fid, err)
					fs.retryState.Delete(n)
					fs.cleanupLocalUploadState(path, n, false)
					return
				}

				attempt := 0
				if v, ok := fs.retryState.Load(n); ok {
					attempt = v.(int)
				}
				attempt++
				fs.retryState.Store(n, attempt)

				if attempt >= maxAutoRetryAttempts {
					driver.Log.Printf("Background Sync Error for %s (fid=%s) reached max retries (%d): %v. Keep pending for manual retry.\n", path, n.fid, attempt, err)
					fs.retryState.Delete(n)
					n.mu.Lock()
					n.syncQueued = false
					n.mu.Unlock()
					return
				}

				delay := time.Duration(attempt*15) * time.Second
				driver.Log.Printf("Background Sync Error for %s (fid=%s): %v. Retrying in %s (attempt %d/%d)...\n", path, n.fid, err, delay, attempt, maxAutoRetryAttempts)
				go func(nodeToRetry *node, d time.Duration) {
					time.Sleep(d)
					fs.enqueueSync(nodeToRetry)
				}(n, delay)
				return
			}

			if stillDirty {
				fs.enqueueSync(n)
			}

			fs.retryState.Delete(n)
			driver.Log.Printf("Successfully synced %s (fid=%s) to Quark Drive\n", path, n.fid)
			n.mu.RLock()
			parentFid := n.parentFid
			n.mu.RUnlock()
			fs.driver.RemoveDirCache(parentFid)
		}(task)
	}
}

func (fs *QryptFS) cleanupLocalUploadState(path string, n *node, isDir bool) {
	if isDir {
		fs.recursiveCleanup(path, n)

		if fs.cache != nil {
			prefix := path
			if !strings.HasSuffix(prefix, "/") {
				prefix += "/"
			}
			if fs.staging != nil {
				pending, err := fs.cache.GetPendingNodes()
				if err == nil {
					for _, entry := range pending {
						if entry.Path == path || strings.HasPrefix(entry.Path, prefix) {
							_ = fs.staging.Remove(entry.LocalPath)
						}
					}
				}
			}
			_ = fs.cache.RemovePendingNodesByPrefix(path)
		}
		return
	}

	fs.deleteNodePath(path, n)
	n.mu.Lock()
	localPath := n.localPath
	n.syncQueued = false
	n.isDirty = false
	n.mu.Unlock()

	fs.retryState.Delete(n)
	if fs.cache != nil {
		_ = fs.cache.RemovePendingNode(path)
		_ = fs.cache.RemovePendingNodesByFid(n.fid)
		_ = fs.cache.RemoveChunksByFid(n.fid)
	}
	if fs.staging != nil {
		_ = fs.staging.Remove(localPath)
	}
}

func (fs *QryptFS) recursiveCleanup(path string, n *node) {
	prefix := path
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	n.mu.RLock()
	type childEntry struct {
		name  string
		child *node
	}
	var children []childEntry
	for name, child := range n.children {
		children = append(children, childEntry{name, child})
	}
	n.mu.RUnlock()

	for _, c := range children {
		fs.recursiveCleanup(prefix+c.name, c.child)
	}

	fs.deleteNodePath(path, n)
	n.mu.Lock()
	n.syncQueued = false
	n.isDirty = false
	n.mu.Unlock()
	fs.retryState.Delete(n)
}

func (fs *QryptFS) maybeSavePendingNodeLocked(path string, n *node, force bool) error {
	if fs.cache == nil {
		return nil
	}

	now := time.Now()
	needSave := force || n.lastPendingSave.IsZero() || now.Sub(n.lastPendingSave) >= pendingNodeSaveInterval
	sizeDelta := n.size - n.lastPendingSize
	if sizeDelta < 0 {
		sizeDelta = -sizeDelta
	}
	if sizeDelta >= pendingNodeSaveSizeStep {
		needSave = true
	}
	if !needSave {
		return nil
	}

	if err := fs.cache.SavePendingNode(path, n.fid, n.parentFid, n.name, n.localPath, n.size, n.isFolder, n.fileNonce[:], n.baseServerMtime, n.baseServerSize); err != nil {
		return err
	}
	n.lastPendingSave = now
	n.lastPendingSize = n.size
	return nil
}

func (fs *QryptFS) enqueueSync(n *node) {
	path := fs.currentPathForNode(n)
	if path == "" {
		return
	}

	n.mu.Lock()
	if !n.isDirty || n.syncQueued || n.localPath == "" {
		n.mu.Unlock()
		return
	}
	if err := fs.maybeSavePendingNodeLocked(path, n, true); err != nil {
		n.mu.Unlock()
		driver.Log.Printf("Failed to save pending node for %s (fid=%s): %v\n", path, n.fid, err)
		return
	}
	n.syncQueued = true
	fid := n.fid
	n.mu.Unlock()

	task := syncTask{node: n}
	select {
	case fs.uploadChan <- task:
		driver.Log.Printf("File %s queued for upload (fid=%s)\n", path, fid)
	default:
		driver.Log.Printf("Upload queue full, blocking for %s (fid=%s)\n", path, fid)
		fs.uploadChan <- task
	}
}

func (fs *QryptFS) notifySyncStart(path string, n *node) {
	if fs.syncObserver == nil {
		return
	}

	n.mu.RLock()
	size := n.size
	n.mu.RUnlock()
	fs.syncObserver.OnSyncStart(path, size)
}

func (fs *QryptFS) notifySyncFinish(snapshot syncPerformanceSnapshot, err error) {
	if fs.syncObserver == nil {
		return
	}
	fs.syncObserver.OnSyncFinish(snapshot, err)
}

func (fs *QryptFS) syncFile(path string, n *node) (err error) {
	startedAt := time.Now()
	stats := syncPerformanceSnapshot{Path: path}
	defer func() {
		stats.TotalDuration = time.Since(startedAt)
		fs.notifySyncFinish(stats, err)
	}()

	n.mu.Lock()
	if !n.isDirty {
		n.mu.Unlock()
		return nil
	}
	snapshotSize := n.size
	snapshotName := n.name
	snapshotMtime := n.mtime
	parentFid := n.parentFid
	fid := n.fid
	baseMtime := n.baseServerMtime
	localPath := n.localPath
	n.mu.Unlock()
	stats.SnapshotSize = snapshotSize

	// 1. Pre-upload Conflict Check
	if !strings.HasPrefix(fid, "local_") {
		rf, err := fs.fileExistsOnServerDetailed(fid, parentFid)
		if err != nil {
			return fmt.Errorf("pre-upload check failed: %v", err)
		}
		if rf == nil {
			// FID is gone. Check if a file with same name exists.
			files, err := fs.driver.ListFiles(parentFid)
			if err == nil {
				var foundRf *driver.File
				for _, f := range files {
					decName, _ := fs.cipher.DecryptSegment(f.FileName)
					if decName == snapshotName {
						foundRf = &f
						break
					}
				}
				if foundRf != nil {
					// Name exists but FID changed: Conflict!
					driver.Log.Printf("Sync: CONFLICT (FID gone but name exists) for %s. Resolving...\n", path)
					fs.resolveConflict(path, n, *foundRf)
					return nil
				}
			}

			// Remote truly deleted, but we have local changes. Convert to local node to re-upload as new file.
			driver.Log.Printf("Sync: remote deleted %s (fid=%s), converting to local node\n", path, fid)
			n.mu.Lock()
			if !strings.HasPrefix(n.fid, "local_") {
				n.fid = "local_" + n.name + "_" + fmt.Sprint(time.Now().UnixNano())
			}
			n.mu.Unlock()
		} else {
			remoteMtime := rf.ModTime().UnixMilli()
			if remoteMtime > baseMtime {
				// Both modified: Diverge!
				driver.Log.Printf("Sync: CONFLICT (both modified) for %s. Remote %v > Base %v. Resolving...\n", path, remoteMtime, baseMtime)
				fs.resolveConflict(path, n, *rf)
				return nil // Task finished as a rename + new path creation
			}
		}
	}

	if fs.staging == nil || fs.uploader == nil || localPath == "" {
		return fmt.Errorf("missing staging state for %s", path)
	}
	snapshotPath, err := fs.staging.Snapshot(localPath)
	if err != nil {
		return err
	}
	defer func() { _ = fs.staging.Remove(snapshotPath) }()

	driver.Log.Printf("Syncing file (Staged): %s (size %d, parentFid %s)\n", snapshotName, snapshotSize, parentFid)
	result, err := fs.uploader.Sync(uploadpkg.SyncRequest{
		Path:      path,
		Name:      snapshotName,
		ParentFid: parentFid,
		LocalPath: snapshotPath,
		PlainSize: snapshotSize,
	})
	stats.PreDuration = result.PreDuration
	stats.UpdateHashDuration = result.UpdateHashDuration
	stats.UploadPartDuration = result.UploadPartDuration
	stats.CommitDuration = result.CommitDuration
	stats.FinishDuration = result.FinishDuration
	stats.PartCount = result.PartCount
	stats.UploadedBytes = result.UploadedBytes
	if err != nil {
		return err
	}

	n.mu.Lock()
	n.fid = result.Fid
	n.fileNonce = result.Nonce
	n.hasNonce = true
	n.encSize = result.EncryptedSize
	if n.mtime.Equal(snapshotMtime) {
		n.isDirty = false
		n.baseServerMtime = time.Now().UnixMilli() // Update base mtime after success
		n.baseServerSize = n.size
		n.lastMetadataCheck = time.Now()
	}
	currentPath := n.currentPath
	localPath = n.localPath
	clearPending := !n.isDirty
	if clearPending {
		n.localPath = ""
	}
	n.mu.Unlock()

	if fs.cache != nil && clearPending {
		_ = fs.cache.RemovePendingNode(path)
		if currentPath != "" && currentPath != path {
			_ = fs.cache.RemovePendingNode(currentPath)
		}
	}
	if clearPending && fs.staging != nil {
		_ = fs.staging.Remove(localPath)
	}
	fs.driver.RemoveDirCache(parentFid)

	return nil
}
