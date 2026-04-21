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

// ensureParentDirExists verifies that the parent directory exists on the server
// and recreates it (recursively) if it was deleted externally.
func (fs *QryptFS) ensureParentDirExists(filePath, parentFid string) error {
	// Root directory always exists
	if parentFid == "" || parentFid == "0" || parentFid == "root" {
		return nil
	}

	// Verify parent exists by checking if its grandparent lists it as a child.
	// We can't use ListFiles(parentFid) because Quark API returns HTTP 200 with
	// empty list for non-existent directories — not an error.
	if fs.dirExistsOnServer(parentFid) {
		return nil
	}

	// Parent doesn't exist on server. We need to recreate the directory path.
	// The parent node may have been removed from fidNodes by MergeRemoteChanges,
	// so we walk up the file path instead of relying on fidNodes.
	type dirSeg struct {
		name     string
		encName  string
		fullPath string
		node     *node // may be nil if node was cleaned up
	}
	var missing []dirSeg

	// Walk up from the file's parent directory, collecting segments.
	dirPath := filepath.Dir(filePath)
	segments := strings.Split(strings.Trim(dirPath, "/"), "/")

	// Build a list of (path, node) for each directory level from root to immediate parent
	type pathLevel struct {
		fullPath string
		segName  string
		node     *node
	}
	var levels []pathLevel
	for i, seg := range segments {
		if seg == "" {
			continue
		}
		p := "/" + strings.Join(segments[:i+1], "/")
		var n *node
		if v, ok := fs.nodes.Load(p); ok {
			n = v.(*node)
		}
		levels = append(levels, pathLevel{fullPath: p, segName: seg, node: n})
	}

	// If file is at mount root (no path segments), the parentFid IS the mount root.
	// Check if it exists on server directly — if not, recreate it under Quark root "0".
	if len(levels) == 0 {
		if parentFid == "" || parentFid == "0" || parentFid == "root" {
			return nil
		}
		driver.Log.Printf("ensureParentDirExists: file at mount root, checking parentFid=%s\n", parentFid)
		if fs.dirExistsOnServer(parentFid) {
			return nil
		}
		// Mount root doesn't exist on server — recreate it
		var mountRootNode *node
		if v, ok := fs.nodes.Load("/"); ok {
			mountRootNode = v.(*node)
		}
		if mountRootNode == nil {
			return fmt.Errorf("mount root node not found in memory for %s", filePath)
		}
		mountRootNode.mu.RLock()
		mountName := mountRootNode.name
		mountRootNode.mu.RUnlock()

		driver.Log.Printf("ensureParentDirExists: mount root dir missing on server, name=%q, oldFid=%s\n", mountName, parentFid)

		if mountName == "" {
			return fmt.Errorf("mount root node has empty name, cannot recreate directory")
		}

		encName := fs.cipher.EncryptSegment(mountName)
		newFid, createErr := fs.driver.CreateDir("0", encName)
		if createErr != nil {
			if strings.Contains(createErr.Error(), "23008") {
				// Dir might already exist — try to find it
				time.Sleep(2 * time.Second)
				fs.driver.RemoveDirCache("0")
				if found, findErr := fs.driver.FindChildByName("0", encName); findErr == nil {
					newFid = found
				} else {
					return fmt.Errorf("failed to recreate mount root dir %s: %v", mountName, createErr)
				}
			} else {
				return fmt.Errorf("failed to recreate mount root dir %s: %v", mountName, createErr)
			}
		}

		driver.Log.Printf("ensureParentDirExists: recreated mount root %s (old fid=%s, new fid=%s)\n", mountName, parentFid, newFid)

		mountRootNode.mu.Lock()
		mountRootNode.fid = newFid
		mountRootNode.parentFid = "0"
		mountRootNode.isDirty = false
		mountRootNode.baseServerMtime = time.Now().UnixMilli()
		mountRootNode.lastMetadataCheck = time.Now()
		mountRootNode.mu.Unlock()
		fs.storeNode("/", mountRootNode)
		return nil
	}

	// Walk from the deepest level upward, finding the deepest ancestor that exists on server
	curFid := ""
	for i := len(levels) - 1; i >= 0; i-- {
		lvl := levels[i]
		if lvl.node == nil {
			// No node in memory — definitely needs recreation
			encName := fs.cipher.EncryptSegment(lvl.segName)
			missing = append(missing, dirSeg{name: lvl.segName, encName: encName, fullPath: lvl.fullPath, node: nil})
			continue
		}

		lvl.node.mu.RLock()
		fid := lvl.node.fid
		isDir := lvl.node.isFolder
		lvl.node.mu.RUnlock()

		if !isDir {
			continue
		}

		if fid == "" || fid == "0" || fid == "root" || fs.dirExistsOnServer(fid) {
			// This directory exists on server — use as anchor
			curFid = fid
			break
		}

		// Doesn't exist on server — needs recreation
		encName := fs.cipher.EncryptSegment(lvl.segName)
		missing = append(missing, dirSeg{name: lvl.segName, encName: encName, fullPath: lvl.fullPath, node: lvl.node})
	}

	if len(missing) == 0 {
		return fmt.Errorf("could not find existing ancestor for parent dir %s of %s", parentFid, filePath)
	}

	if curFid == "" || curFid == "root" {
		// Reached the mount root — use "0" as the base
		curFid = "0"
	}

	// Reverse: walk from existing ancestor down to the immediate parent
	for i, j := 0, len(missing)-1; i < j; i, j = i+1, j-1 {
		missing[i], missing[j] = missing[j], missing[i]
	}

	driver.Log.Printf("ensureParentDirExists: recreating %d missing directories for %s\n", len(missing), filePath)

	// curFid now points to the deepest ancestor that exists on the server.
	// Recreate each missing directory level.
	for _, seg := range missing {
		fs.driver.RemoveDirCache(curFid)
		newFid, createErr := fs.driver.CreateDir(curFid, seg.encName)
		if createErr != nil {
			if strings.Contains(createErr.Error(), "23008") {
				// Transient conflict — directory might be creating. Retry with lookup.
				time.Sleep(2 * time.Second)
				fs.driver.RemoveDirCache(curFid)
				if found, findErr := fs.driver.FindChildByName(curFid, seg.encName); findErr == nil {
					newFid = found
				} else {
					newFid, createErr = fs.driver.CreateDir(curFid, seg.encName)
					if createErr != nil {
						return fmt.Errorf("failed to recreate dir %s: %v", seg.name, createErr)
					}
				}
			} else {
				return fmt.Errorf("failed to recreate dir %s: %v", seg.name, createErr)
			}
		}

		driver.Log.Printf("ensureParentDirExists: recreated dir %s (new fid=%s, parent=%s)\n",
			seg.name, newFid, curFid)

		// Update or create the in-memory node for this directory
		if seg.node != nil {
			seg.node.mu.Lock()
			seg.node.fid = newFid
			seg.node.parentFid = curFid
			seg.node.isDirty = false
			seg.node.baseServerMtime = time.Now().UnixMilli()
			seg.node.lastMetadataCheck = time.Now()
			seg.node.mu.Unlock()
			fs.storeNode(seg.node.currentPath, seg.node)
		} else {
			// Original node was cleaned up — create a new one
			newNode := &node{
				fid:               newFid,
				parentFid:         curFid,
				name:              seg.name,
				currentPath:       seg.fullPath,
				isFolder:          true,
				mtime:             time.Now(),
				baseServerMtime:   time.Now().UnixMilli(),
				lastMetadataCheck: time.Now(),
			}
			fs.storeNode(seg.fullPath, newNode)
			driver.Log.Printf("ensureParentDirExists: created new node for %s (fid=%s)\n", seg.fullPath, newFid)
		}

		fs.driver.RemoveDirCache(curFid)
		curFid = newFid
	}

	return nil
}

// dirExistsOnServer checks if a directory with the given fid exists by verifying
// it as a child of its own parent node. This avoids the ListFiles quirk where
// Quark API returns HTTP 200 with empty list for non-existent directories.
func (fs *QryptFS) dirExistsOnServer(dirFid string) bool {
	if dirFid == "" || dirFid == "0" || dirFid == "root" {
		return true
	}
	v, ok := fs.fidNodes.Load(dirFid)
	if !ok {
		return false
	}
	n := v.(*node)
	n.mu.RLock()
	pFid := n.parentFid
	encName := fs.cipher.EncryptSegment(n.name)
	n.mu.RUnlock()

	if pFid == "" || pFid == "0" || pFid == "root" {
		// Mount root — always exists
		return true
	}

	_, err := fs.driver.FindChildByName(pFid, encName)
	return err == nil
}

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

	// OPTIMIZATION: If parent's children are fresh (within TTL), check in-memory
	// instead of making an API call. This avoids RemoveDirCache + ListFiles every time.
	if parentNode, ok := fs.fidNodes.Load(parentFid); ok {
		pn := parentNode.(*node)
		pn.mu.RLock()
		lastCheck := pn.lastMetadataCheck
		if time.Since(lastCheck) < MetadataTTL {
			// Cache is fresh — search children by fid
			for _, child := range pn.children {
				child.mu.RLock()
				childFid := child.fid
				child.mu.RUnlock()
				if childFid == fid {
					pn.mu.RUnlock()
					return &driver.File{Fid: fid}, nil // File exists in cache
				}
			}
			pn.mu.RUnlock()
			// FID not in local children, but cache is fresh.
			// This can happen if a file was uploaded externally (bypassing FUSE).
			// Fall through to server check instead of assuming file is gone.
			driver.Log.Printf("fileExistsOnServerDetailed: fid=%s not in local children of parent=%s, checking server\n", fid, parentFid)
			goto serverCheck
		}
		pn.mu.RUnlock()
	}

	// Fallback: cache is stale or missing, fetch from server
serverCheck:
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

			if attempt >= fs.maxRetries {
				driver.Log.Printf("Background Sync Error for %s (fid=%s) reached max retries (%d): %v. Keep pending for manual retry.\n", path, n.fid, attempt, err)
				fs.retryState.Delete(n)
				n.mu.Lock()
				n.syncQueued = false
				n.mu.Unlock()
				return
			}

			delay := time.Duration(attempt*15) * time.Second
			driver.Log.Printf("Background Sync Error for %s (fid=%s): %v. Retrying in %s (attempt %d/%d)...\n", path, n.fid, err, delay, attempt, fs.maxRetries)
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

	// 1b. Check for same-name conflict on server (external upload with different FID)
	if !strings.HasPrefix(fid, "local_") {
		files, listErr := fs.driver.ListFiles(parentFid)
		if listErr == nil {
			for _, f := range files {
				if f.Fid == fid {
					continue // our own file, skip
				}
				decName, _ := fs.cipher.DecryptSegment(f.FileName)
				if decName == snapshotName {
					// Same name but different FID — external modification detected
					driver.Log.Printf("Sync: CONFLICT (same name, different FID) for %s. Our FID=%s, server FID=%s. Resolving...\n", path, fid, f.Fid)
					fs.resolveConflict(path, n, f)
					return nil
				}
			}
		}
	}

	// 1c. Pre-upload Conflict Check (existing FID-based check)
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
			if remoteMtime > baseMtime+2000 {
				// Both modified: Diverge!
				driver.Log.Printf("Sync: CONFLICT (both modified) for %s. Remote %v > Base %v (grace 2s). Resolving...\n", path, remoteMtime, baseMtime)
				fs.resolveConflict(path, n, *rf)
				return nil // Task finished as a rename + new path creation
			}
		}
	}

	// 1d. Ensure parent directory exists on server (handles external deletion)
	if err := fs.ensureParentDirExists(path, parentFid); err != nil {
		return fmt.Errorf("parent dir check failed for %s: %v", path, err)
	}
	// Parent may have been recreated with a new FID — refresh parentFid from node tree
	parentDirPath := filepath.Dir(path)
	if pv, ok := fs.nodes.Load(parentDirPath); ok {
		pn := pv.(*node)
		pn.mu.RLock()
		newParentFid := pn.fid
		pn.mu.RUnlock()
		if newParentFid != parentFid {
			driver.Log.Printf("syncFile: parentFid updated for %s: %s -> %s\n", path, parentFid, newParentFid)
			parentFid = newParentFid
			n.mu.Lock()
			n.parentFid = newParentFid
			n.mu.Unlock()
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

	// Verify staging file size matches node.size to prevent uploading incomplete files.
	// FUSE may call Release before all Writes complete (e.g., macFUSE behavior).
	actualSize, sizeErr := fs.staging.FileSize(snapshotPath)
	if sizeErr != nil {
		return fmt.Errorf("failed to stat snapshot for %s: %v", path, sizeErr)
	}
	driver.Log.Printf("syncFile: snapshot ready for %s: node.size=%d, actualSize=%d, localPath=%s\n", path, snapshotSize, actualSize, localPath)
	// Safety: never upload when staging file is empty but node thinks there's data.
	// This catches double-sync bugs where first sync deleted the staging file.
	if actualSize == 0 && snapshotSize > 0 {
		driver.Log.Printf("Sync: staging file empty but node.size=%d for %s — likely double-sync. Re-queuing...\n", snapshotSize, path)
		n.mu.Lock()
		n.syncQueued = false
		n.mu.Unlock()
		return fmt.Errorf("staging file empty but node.size=%d for %s", snapshotSize, path)
	}
	if snapshotSize > 0 && actualSize < snapshotSize {
		driver.Log.Printf("Sync: WARNING: staging file size %d < expected %d for %s. Re-queuing...\n", actualSize, snapshotSize, path)
		n.mu.Lock()
		n.syncQueued = false
		n.mu.Unlock()
		return fmt.Errorf("staging file incomplete (%d < %d)", actualSize, snapshotSize)
	}

	driver.Log.Printf("Syncing file (Staged): %s (size %d, parentFid %s)\n", snapshotName, snapshotSize, parentFid)

	// Final check: if the original staging file has grown since our snapshot,
	// more writes arrived after we captured — re-queue to get the complete data.
	if currentSize, err := fs.staging.FileSize(localPath); err == nil && currentSize > actualSize {
		driver.Log.Printf("Sync: staging file grew from %d to %d during sync for %s — re-queuing\n", actualSize, currentSize, path)
		n.mu.Lock()
		n.syncQueued = false
		n.mu.Unlock()
		return fmt.Errorf("staging file grew during sync (%d > %d)", currentSize, actualSize)
	}

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
		// 使用本地文件的 mtime 作为 base，避免 MergeRemoteChanges 误判冲突
		n.baseServerMtime = snapshotMtime.UnixMilli()
		n.baseServerSize = n.size
		n.lastMetadataCheck = time.Now()
		n.lastUploadTime = time.Now() // 记录上传完成时间，防止 API 索引延迟导致误删
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

	return nil
}
