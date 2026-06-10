//go:build !nofuse

package fusefs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
	upload "github.com/yinzhenyu/skills/qrypt/internal/upload"
)

var errCooldown = errors.New("upload cooldown active")

// collectAncestorPaths returns ancestor directory paths for a node,
// starting from its parent up to the root.
func collectAncestorPaths(n *Node) []string {
	dir := filepath.Dir(n.currentPath)
	var paths []string
	for {
		paths = append(paths, dir)
		if dir == "/" {
			break
		}
		dir = filepath.Dir(dir)
	}
	return paths
}

// incParentUploadCounts increments uploadingChildren on every ancestor
// of n, preventing Rmdir on parent directories during upload.
func (fs *QryptFS) incParentUploadCounts(n *Node) {
	for _, p := range collectAncestorPaths(n) {
		if v, ok := fs.nodes.Load(p); ok {
			atomic.AddInt32(&v.(*Node).uploadingChildren, 1)
		}
	}
}

// decParentUploadCounts decrements uploadingChildren on every ancestor.
func (fs *QryptFS) decParentUploadCounts(n *Node) {
	for _, p := range collectAncestorPaths(n) {
		if v, ok := fs.nodes.Load(p); ok {
			atomic.AddInt32(&v.(*Node).uploadingChildren, -1)
		}
	}
}

// enqueueNode enqueues a node for upload via the active queue (orchestrator or uploadChan).
func (fs *QryptFS) enqueueNode(n *Node) {
	if fs.uploadQueue != nil {
		path := n.currentPath
		fs.uploadQueue.Submit(func(ctx context.Context) error {
			return fs.syncFile(path, n)
		})
	} else {
		fs.uploadChan <- syncTask{node: n}
	}
}

func (fs *QryptFS) enqueueSync(n *Node) {
	fs.enqueueSyncDelay(n, 0)
}

func (fs *QryptFS) enqueueSyncDelay(n *Node, delay time.Duration) {
	n.mu.Lock()
	isNewLocal := strings.HasPrefix(n.fid, "local_")
	if !n.isDirty && !isNewLocal {
		n.mu.Unlock()
		return
	}
	if n.syncQueued {
		n.mu.Unlock()
		return
	}
	n.syncQueued = true
	path := n.currentPath
	n.mu.Unlock()

	if delay > 0 {
		fs.syncDelayMu.Lock()
		if t, ok := fs.syncTimers[path]; ok {
			t.Stop()
		}
		fs.syncTimers[path] = time.AfterFunc(delay, func() {
			fs.syncDelayMu.Lock()
			delete(fs.syncTimers, path)
			fs.syncDelayMu.Unlock()
			if fs.IsShuttingDown() {
				return
			}
			// Look up current node by path — the node may have been
			// recreated by MergeRemoteChanges since the timer was set.
			if node, _ := fs.nodes.Load(path); node != nil {
				fs.enqueueNode(node.(*Node))
			}
		})
		fs.syncDelayMu.Unlock()
	} else {
		fs.enqueueNode(n)
	}
}

func (fs *QryptFS) cancelSyncTimer(path string) {
	fs.syncDelayMu.Lock()
	if t, ok := fs.syncTimers[path]; ok {
		t.Stop()
		delete(fs.syncTimers, path)
	}
	fs.syncDelayMu.Unlock()
}

func (fs *QryptFS) syncFile(path string, n *Node) (err error) {
	logging.L.Infof("syncFile: starting sync for %s\n", path)

	if fs.isUnderDeletingDir(path) {
		logging.L.Infof("syncFile: aborting sync for %s (being deleted)\n", path)
		n.mu.Lock()
		n.syncQueued = false
		n.mu.Unlock()
		return nil
	}
	if n.IsCancelled() {
		logging.L.Infof("syncFile: aborting sync for %s (node cancelled)\n", path)
		n.mu.Lock()
		n.syncQueued = false
		n.mu.Unlock()
		return nil
	}

	n.mu.RLock()
	currentPath := n.currentPath
	n.mu.RUnlock()
	if currentPath == "" || currentPath != path {
		logging.L.Infof("syncFile: aborting sync for %s (detached or path changed)\n", path)
		n.mu.Lock()
		n.syncQueued = false
		n.mu.Unlock()
		return nil
	}

	// ── Upload in progress ──────────────────────────────────────────
	// Increment uploadingChildren on ancestors to prevent Rmdir
	// while upload is in flight.
	ancestorPaths := collectAncestorPaths(n)
	for _, p := range ancestorPaths {
		if v, ok := fs.nodes.Load(p); ok {
			atomic.AddInt32(&v.(*Node).uploadingChildren, 1)
		}
	}
	defer func() {
		for _, p := range ancestorPaths {
			if v, ok := fs.nodes.Load(p); ok {
				atomic.AddInt32(&v.(*Node).uploadingChildren, -1)
			}
		}
	}()

	// ┌──────────────────────────────────────────────────────────────┐
	// │ WARNING: syncQueued lifecycle — concurrent-worker dupe guard │
	// │                                                              │
	// │ The syncQueued flag prevents multiple tasks for the same     │
	// │ node from being queued.  The defer below MUST NOT clear the  │
	// │ flag when re-enqueuing — doing so creates a TOCTOU window    │
	// │ where a concurrent Release can queue a second task, letting  │
	// │ two workers upload the same file simultaneously. The Quark   │
	// │ API then auto-renames the second upload → file(1).txt dupes. │
	// │                                                              │
	// │ The re-enqueue sends directly to uploadChan (bypassing       │
	// │ enqueueSyncDelay) because syncQueued is already true.        │
	// └──────────────────────────────────────────────────────────────┘
	defer func() {
		n.mu.Lock()
		isStillDirty := n.isDirty
		if isStillDirty && err == nil {
			n.mu.Unlock()
			fs.enqueueNode(n)
		} else {
			n.syncQueued = false
			n.mu.Unlock()
		}
	}()

	n.mu.Lock()
	if !n.isDirty {
		n.mu.Unlock()
		return nil
	}
	if n.localPath != "" && fs.staging != nil {
		if actualSize, err := fs.staging.FileSize(n.localPath); err == nil {
			if actualSize > 0 && n.size != actualSize {
				n.size = actualSize
			}
		}
	}

	// ── Snapshot: COW copy of staging at sync start ───────────────
	// Take a copy-on-write snapshot so the upload process reads a
	// consistent file state, while concurrent Writes continue to the
	// live staging file unaffected.
	snapshotName := n.name
	parentFid := n.parentFid
	snapshotSize := n.size
	oldUploadedFid := n.uploadedFid
	uploadID := n.uploadID
	lastPart := n.lastPart
	localPath := n.localPath
	var snapPath string
	if localPath != "" && fs.staging != nil {
		var snapErr error
		snapPath, snapErr = fs.staging.Snapshot(localPath)
		if snapErr != nil {
			logging.L.Warnf("syncFile: snapshot failed for %s: %v (falling back to live file)", path, snapErr)
			snapPath = localPath
		}
	}
	if snapPath != "" && snapPath != localPath {
		defer fs.staging.ReleaseSnapshot(snapPath)
	}
	uploadFid := n.fid
	lastUpload := n.lastUploadTime
	n.mu.Unlock()

	// ── 10s upload cooldown ───────────────────────────────────────
	if !strings.HasPrefix(uploadFid, "local_") && !lastUpload.IsZero() && time.Since(lastUpload) < 10*time.Second {
		go func() {
			time.Sleep(time.Until(lastUpload.Add(10 * time.Second)))
			if !fs.IsShuttingDown() {
				fs.enqueueSync(n)
			}
		}()
		return errCooldown
	}

	// ── Check parent directory ─────────────────────────────────────
	// For new files (local_ fid), ensure the parent directory exists
	// on the server before uploading.
	if strings.HasPrefix(uploadFid, "local_") {
		if err := fs.ensureParentDirExists(path, parentFid); err != nil {
			return fmt.Errorf("failed to ensure parent dir: %v", err)
		}
		// After ensureParentDirExists, the parent directory's fid has been
		// updated from local_ to a real cloud fid. Update the file's parentFid
		// to match, so the upload targets the correct remote parent.
		parentPath := filepath.Dir(path)
		if parentNode, errc := fs.lookup(parentPath); errc == 0 {
			parentNode.mu.RLock()
			parentFid = parentNode.fid
			parentNode.mu.RUnlock()
			n.mu.Lock()
			n.parentFid = parentFid
			n.mu.Unlock()
		}
	}

	// ── Delete previous uploaded file by fid ──────────────────────
	// Quark API index delay means deleteExistingFileByName (inside
	// Put) may not find the old file, causing server auto-rename to
	// name(1).  Use a direct fid-based delete instead.
	if oldUploadedFid != "" && !strings.HasPrefix(oldUploadedFid, "local_") {
		if w, ok := fs.drv.(drivers.Writer); ok {
			if err := w.Remove(context.Background(), drivers.Entry{ID: oldUploadedFid}); err != nil {
				logging.L.Warnf("syncFile: remove old file %s: %v", oldUploadedFid, err)
			}
		}
	}

	// ── Re-check current path ─────────────────────────────────────
	// The file may have been renamed between the snapshot and now.
	// Re-read name + parentFid so the upload targets the correct
	// location, avoiding a post-upload Move attempt.
	n.mu.RLock()
	currentParentFid := n.parentFid
	currentName := n.name
	n.mu.RUnlock()
	if currentParentFid != parentFid || currentName != snapshotName {
		parentFid = currentParentFid
		snapshotName = currentName
	}

	uploadCtx, uploadCancel := context.WithTimeout(drivers.WithMtime(context.Background(), n.mtime), 30*time.Minute)
	defer uploadCancel()
	uploadReader := func() (io.ReadCloser, error) {
		if snapPath == "" {
			return nil, fmt.Errorf("staging file path is empty")
		}
		return fs.staging.OpenReader(snapPath)
	}
	result, err := fs.uploader.Upload(uploadCtx, upload.Request{
		Path:       path,
		Name:       snapshotName,
		ParentFid:  parentFid,
		PlainSize:  snapshotSize,
		OldFid:     oldUploadedFid,
		Nonce:      n.fileNonce,
		UploadID:   uploadID,
		LastPart:   lastPart,
		DataReader: uploadReader,
		ProgressFn: func(partNumber int) {
			n.mu.Lock()
			if n.lastPart < partNumber {
				n.lastPart = partNumber
			}
			n.mu.Unlock()
			if partNumber%25 == 0 && fs.cacheMgr != nil {
				fs.cacheMgr.UpdatePendingNodeLastPart(path, partNumber)
			}
		},
	})

	if result.UploadID != "" && fs.cacheMgr != nil {
		n.mu.Lock()
		n.uploadID = result.UploadID
		n.mu.Unlock()
		fs.cacheMgr.UpdatePendingNodeUpload(path, result.UploadID)
	}

	if err != nil {
		return err
	}

	n.mu.Lock()
	oldFid := n.fid
	n.fid = result.Fid
	n.expectedFid = result.Fid
	n.fileNonce = result.Nonce
	n.hasNonce = true
	n.encSize = result.EncryptedSize
	n.uploadedFid = result.Fid
	n.isDirty = false
	n.baseServerMtime = time.Now().UnixMilli()
	n.baseServerSize = n.size
	n.lastMetadataCheck = time.Now()
	n.lastUploadTime = time.Now()
	n.uploadID = ""
	n.lastPart = 0
	localPath = n.localPath
	if fs.cacheMgr != nil {
		fs.cacheMgr.RemovePendingNode(path)
		if n.currentPath != "" && n.currentPath != path {
			fs.cacheMgr.RemovePendingNode(n.currentPath)
		}
	}
	n.localPath = ""
	if fs.staging != nil && localPath != "" {
		fs.staging.Remove(localPath)
	}
	newFid := n.fid
	n.mu.Unlock()

	fs.syncFilePostUpload(path, n, newFid, oldFid, snapshotName, parentFid)
	return nil
}

func (fs *QryptFS) syncFilePostUpload(path string, n *Node, newFid, oldFid, snapshotName, parentFid string) {
	w, wOk := fs.drv.(drivers.Writer)

	if newFid != "" && !strings.HasPrefix(newFid, "local_") {
		n.mu.RLock()
		curParentFid := n.parentFid
		curName := n.name
		n.mu.RUnlock()

		if curParentFid != "" && curParentFid != "0" &&
			parentFid != "" && parentFid != "0" &&
			curParentFid != parentFid &&
			!strings.HasPrefix(parentFid, "local_") &&
			!strings.HasPrefix(curParentFid, "local_") {
			logging.L.Infof("syncFilePostUpload: moving %s (fid=%s) from parent %s to %s\n", path, newFid, parentFid, curParentFid)
			if wOk {
				moveEntry := drivers.Entry{ID: newFid, ParentID: parentFid}
				var moveErr error
				for attempt := 0; attempt < 3; attempt++ {
					moveErr = w.Move(context.Background(), moveEntry, curParentFid)
					if moveErr == nil {
						break
					}
					if errors.Is(moveErr, drivers.ErrDirAlreadyExists) || strings.Contains(moveErr.Error(), "conflict") {
						time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
						continue
					}
					break
				}
				if moveErr != nil {
					logging.L.Errorf("syncFilePostUpload: move failed for %s: %v\n", path, moveErr)
				} else {
					logging.L.Debugf("syncFilePostUpload: move succeeded\n")
				}
			}
		} else if curParentFid != parentFid {
			logging.L.Debugf("syncFilePostUpload: parentFid changed but skipping move (local_ prefix): %s -> %s\n", parentFid, curParentFid)
		}

		if curName != snapshotName {
			encName := fs.cp.EncryptSegment(curName)
			logging.L.Infof("syncFilePostUpload: renaming %s from %s to %s\n", path, snapshotName, curName)
			if wOk {
				renameEntry := drivers.Entry{ID: newFid, ParentID: curParentFid}
				var renameErr error
				for attempt := 0; attempt < 3; attempt++ {
					renameErr = w.Rename(context.Background(), renameEntry, encName)
					if renameErr == nil {
						break
					}
					if errors.Is(renameErr, drivers.ErrDirAlreadyExists) || strings.Contains(renameErr.Error(), "conflict") {
						time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
						continue
					}
					break
				}
				if renameErr != nil {
					logging.L.Errorf("syncFilePostUpload: rename failed for %s: %v\n", path, renameErr)
				}
			}
		}
	}

	if oldFid != "" && oldFid != newFid {
		logging.L.Debugf("syncFilePostUpload: replacing fid index %s -> %s for %s\n", oldFid, newFid, path)
		fs.fidNodes.Delete(oldFid)
		if fs.cacheMgr != nil {
			fs.cacheMgr.RemoveChunksByFid(oldFid)
		}
	}
	if newFid != "" && !strings.HasPrefix(newFid, "local_") {
		fs.fidNodes.Store(newFid, n)
	}
}

func (fs *QryptFS) fileExistsOnServerDetailed(fid, parentFid string) (*drivers.Entry, error) {
	if fid == "" || strings.HasPrefix(fid, "local_") {
		return nil, nil
	}
	files, err := fs.drv.List(context.Background(), parentFid)
	if err != nil {
		if errors.Is(err, drivers.ErrNotFound) || strings.Contains(err.Error(), "404") {
			return nil, nil
		}
		return nil, err
	}
	for _, f := range files {
		if f.ID == fid {
			return &f, nil
		}
	}
	return nil, nil
}

func (fs *QryptFS) ensureParentDirExists(filePath, parentFid string) error {
	if parentFid == "" || parentFid == "0" || parentFid == "root" {
		return nil
	}

	if fs.dirExistsOnServer(parentFid) {
		return nil
	}
	logging.L.Infof("ensureParentDirExists: parent not found on server for %s (parentFid=%s)\n", filePath, parentFid)

	dirPath := filepath.Dir(filePath)
	segments := strings.Split(strings.Trim(dirPath, "/"), "/")

	type pathLevel struct {
		fullPath string
		segName  string
		node     *Node
	}
	var levels []pathLevel
	for i, seg := range segments {
		if seg == "" {
			continue
		}
		p := "/" + strings.Join(segments[:i+1], "/")
		var n *Node
		if v, ok := fs.nodes.Load(p); ok {
			n = v.(*Node)
		}
		levels = append(levels, pathLevel{fullPath: p, segName: seg, node: n})
	}

	if len(levels) == 0 {
		if parentFid == "" || parentFid == "0" || parentFid == "root" {
			return nil
		}
		if fs.dirExistsOnServer(parentFid) {
			return nil
		}
		var mountRootNode *Node
		if v, ok := fs.nodes.Load("/"); ok {
			mountRootNode = v.(*Node)
		}
		if mountRootNode == nil {
			return fmt.Errorf("mount root node not found for %s", filePath)
		}
		mountRootNode.mu.RLock()
		mountName := mountRootNode.name
		mountRootNode.mu.RUnlock()

		if mountName == "" {
			return fmt.Errorf("mount root node has empty name")
		}

		encName := fs.cp.EncryptSegment(mountName)
		newFid, createErr := fs.ensureRemoteDir("0", encName)
		if createErr != nil {
			return createErr
		}
		mountRootNode.mu.Lock()
		mountRootNode.fid = newFid
		mountRootNode.mu.Unlock()
		fs.fidNodes.Store(newFid, mountRootNode)
		return nil
	}

	currentRemoteParentFid := "0"
	for _, level := range levels {
		encName := fs.cp.EncryptSegment(level.segName)
		fid, err := fs.findChildDir(context.Background(), currentRemoteParentFid, encName)
		if err != nil {
			newFid, createErr := fs.ensureRemoteDir(currentRemoteParentFid, encName)
			if createErr != nil {
				return createErr
			}
			fid = newFid
		}

		if level.node != nil {
			level.node.mu.Lock()
			oldFid := level.node.fid
			level.node.fid = fid
			level.node.parentFid = currentRemoteParentFid
			level.node.mu.Unlock()

			if oldFid != fid {
				if oldFid != "" {
					fs.fidNodes.Delete(oldFid)
				}
				fs.fidNodes.Store(fid, level.node)
			}
		}
		currentRemoteParentFid = fid
	}

	return nil
}

func (fs *QryptFS) dirExistsOnServer(fid string) bool {
	if fid == "" || fid == "0" || fid == "root" {
		return true
	}
	// Local-only fids haven't been synced to the cloud yet.
	if strings.HasPrefix(fid, "local_") {
		return false
	}
	_, err := fs.drv.List(context.Background(), fid)
	if err != nil {
		if errors.Is(err, drivers.ErrNotFound) || strings.Contains(err.Error(), "404") {
			return false
		}
	}
	return true
}

func (fs *QryptFS) findChildDir(ctx context.Context, parentFid, encName string) (string, error) {
	entries, err := fs.drv.List(ctx, parentFid)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.Name == encName && e.IsDir {
			return e.ID, nil
		}
	}
	return "", fmt.Errorf("child dir not found: %s", encName)
}

func (fs *QryptFS) ensureRemoteDir(parentFid, encName string) (string, error) {
	w, ok := fs.drv.(drivers.Writer)
	if !ok {
		return "", fmt.Errorf("driver does not support write operations")
	}
	entry, err := w.Mkdir(context.Background(), parentFid, encName)
	if err == nil {
		return entry.ID, nil
	}
	if errors.Is(err, drivers.ErrDirAlreadyExists) {
		return fs.findChildDir(context.Background(), parentFid, encName)
	}
	return "", err
}

func (fs *QryptFS) cleanupLocalUploadState(path string, n *Node, recursive bool) {
	if n == nil {
		return
	}
	n.mu.RLock()
	fid := n.fid
	localPath := n.localPath
	n.mu.RUnlock()

	if fs.cacheMgr != nil {
		if recursive {
			fs.cacheMgr.RemovePendingNodesByPrefix(path)
		} else {
			fs.cacheMgr.RemovePendingNode(path)
			if fid != "" {
				fs.cacheMgr.RemovePendingNodesByFid(fid)
			}
		}
	}
	if fs.staging != nil && localPath != "" {
		fs.staging.Remove(localPath)
	}
}
