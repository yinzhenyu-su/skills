//go:build !nofuse

package fs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/drive"
	"github.com/yinzhenyu/skills/qrypt/internal/log"
	syncpkg "github.com/yinzhenyu/skills/qrypt/internal/sync"
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
	n.mu.RLock()
	isNewLocal := strings.HasPrefix(n.fid, "local_")
	if !n.isDirty && !isNewLocal {
		n.mu.RUnlock()
		return
	}
	n.mu.RUnlock()

	if delay > 0 {
		n.mu.Lock()
		n.syncQueued = true
		n.mu.Unlock()

		path := n.currentPath
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
		n.mu.Lock()
		n.syncQueued = true
		n.mu.Unlock()
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
	log.L.Infof("syncFile: starting sync for %s\n", path)

	if fs.isUnderDeletingDir(path) {
		log.L.Infof("syncFile: aborting sync for %s (being deleted)\n", path)
		n.mu.Lock()
		n.syncQueued = false
		n.mu.Unlock()
		return nil
	}
	if n.IsCancelled() {
		log.L.Infof("syncFile: aborting sync for %s (node cancelled)\n", path)
		n.mu.Lock()
		n.syncQueued = false
		n.mu.Unlock()
		return nil
	}

	n.mu.RLock()
	currentPath := n.currentPath
	n.mu.RUnlock()
	if currentPath == "" || currentPath != path {
		log.L.Infof("syncFile: aborting sync for %s (detached or path changed)\n", path)
		n.mu.Lock()
		n.syncQueued = false
		n.mu.Unlock()
		return nil
	}

	// ── Edit-protection lock ──────────────────────────────────────
	// All early-exit checks passed. Set uploading = 1 so concurrent
	// Write / Truncate / Unlink / Rename return EBUSY.
	// Also increment uploadingChildren on ancestors to prevent Rmdir.
	atomic.StoreInt32(&n.uploading, 1)
	ancestorPaths := collectAncestorPaths(n)
	for _, p := range ancestorPaths {
		if v, ok := fs.nodes.Load(p); ok {
			atomic.AddInt32(&v.(*Node).uploadingChildren, 1)
		}
	}
	needsUnlock := true
	defer func() {
		if needsUnlock {
			for _, p := range ancestorPaths {
				if v, ok := fs.nodes.Load(p); ok {
					atomic.AddInt32(&v.(*Node).uploadingChildren, -1)
				}
			}
			atomic.StoreInt32(&n.uploading, 0)
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

	// ── Snapshot (lock-protected, used after unlock) ──────────────
	// These values reflect the file state at syncFile start and are
	// used later for the upload request (snapshotSize, snapshotName,
	// parentFid), conflict detection (fid), and cleanup (localPath).
	// The 10s delay below can cause fid to diverge from n.fid — see
	// • currentFid re-read just before conflict detection.
	snapshotSize := n.size
	snapshotName := n.name
	parentFid := n.parentFid
	fid := n.fid
	localPath := n.localPath
	lastUpload := n.lastUploadTime
	oldUploadedFid := n.uploadedFid
	uploadID := n.uploadID
	lastPart := n.lastPart
	n.mu.Unlock()

	// ── 10s upload cooldown ───────────────────────────────────────
	// Prevents rapid re-uploads after a successful upload.  Must NOT
	// return nil — doing so makes the re-enqueue defer re-queue
	// immediately, creating a tight loop (isStillDirty && err == nil).
	// Instead return a sentinel error (syncQueued is cleared), and
	// schedule a delayed re-enqueue.
	if !strings.HasPrefix(fid, "local_") && !lastUpload.IsZero() && time.Since(lastUpload) < 10*time.Second {
		go func() {
			time.Sleep(time.Until(lastUpload.Add(10 * time.Second)))
			if !fs.IsShuttingDown() {
				fs.enqueueSync(n)
			}
		}()
		return errCooldown
	}

	// ── Refresh n.fid after 10s delay ─────────────────────────────
	// The snapshot fid above was captured BEFORE the 10s guard, so
	// it may point to a server entry that a PRIOR syncFile's upload
	// already deleted-and-replaced (new fid).  If we used the stale
	// fid in the conflict checks below, every ListFiles hit would
	// look like "file exists but fid differs" → false conflict.
	n.mu.RLock()
	currentFid := n.fid
	n.mu.RUnlock()

	// ┌──────────────────────────────────────────────────────────────┐
	// │ CONFLICT DETECTION (against server parent dir)               │
	// │                                                              │
	// │ Both checks use currentFid (freshly re-read) rather than the │
	// │ snapshot fid.  The 10s guard can delay execution long enough │
	// │ for a prior syncFile on another worker to complete an upload │
	// │ (deleteExistingFileByName → UploadPre → upload parts → fin) │
	// │ which changes n.fid AND deletes the old fid from the server. │
	// │                                                              │
	// │ Check 1 — name match, different fid:                         │
	// │   A file with the same decrypted name exists in the parent   │
	// │   dir but its fid does not match currentFid.  This means     │
	// │   someone else uploaded a file with the same name → conflict.│
	// │                                                              │
	// │ Check 2 — our fid gone, name exists:                         │
	// │   currentFid is not found on the server at all.  This means  │
	// │   the file was deleted remotely.  If another file with the   │
	// │   same name exists, treat it as conflict.  Otherwise reset   │
	// │   fid to local_ to upload as a brand-new file.               │
	// │                                                              │
	// │ NOTE: There is deliberately NO mtime comparison here — this  │
	// │ is an encrypted filesystem where the server's recorded mtime │
	// │ (upload completion) never matches the client's mtime (save   │
	// │ time), producing false positives with clock skew. The two    │
	// │ fid-based checks above catch every legitimate conflict.      │
	// └──────────────────────────────────────────────────────────────┘
	if !strings.HasPrefix(fid, "local_") && !strings.HasPrefix(currentFid, "local_") &&
		(lastUpload.IsZero() || time.Since(lastUpload) > 30*time.Second) {
		files, listErr := fs.drv.List(context.Background(), parentFid)
		if listErr == nil {
			for _, f := range files {
				if f.ID == currentFid {
					continue
				}
				decName, _ := fs.cipher.DecryptSegment(f.Name)
				if decName == snapshotName {
					fs.resolveConflict(path, n, f)
					return nil
				}
			}
		}
	}

	if !strings.HasPrefix(fid, "local_") && !strings.HasPrefix(currentFid, "local_") {
		rf, err := fs.fileExistsOnServerDetailed(currentFid, parentFid)
		if err != nil {
			return fmt.Errorf("pre-upload check failed: %v", err)
		}
		if rf == nil {
			files, err := fs.drv.List(context.Background(), parentFid)
			if err == nil {
				for _, f := range files {
					decName, _ := fs.cipher.DecryptSegment(f.Name)
					if decName == snapshotName {
						fs.resolveConflict(path, n, f)
						return nil
					}
				}
			}
			fid = "local_" + n.name
		}
	}

	if strings.HasPrefix(fid, "local_") {
		err = fs.ensureParentDirExists(path, parentFid)
		if err != nil {
			return fmt.Errorf("failed to ensure parent dir: %v", err)
		}
		n.mu.RLock()
		parentFid = n.parentFid
		n.mu.RUnlock()
	}

	// Delete previous uploaded file by its known fid (not by name listing).
	// Quark API index delay means deleteExistingFileByName (inside Put) may
	// not find the old file, causing the server to auto-rename to name(1).
	if oldUploadedFid != "" && !strings.HasPrefix(oldUploadedFid, "local_") {
		if w, ok := fs.drv.(drive.Writer); ok {
			if err := w.Remove(context.Background(), drive.Entry{ID: oldUploadedFid}); err != nil {
				log.L.Warnf("syncFile: remove old file %s: %v", oldUploadedFid, err)
			}
		}
	}

	uploadCtx, uploadCancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer uploadCancel()
	result, err := fs.uploader.Upload(uploadCtx, syncpkg.Request{
		Path:      path,
		Name:      snapshotName,
		ParentFid: parentFid,
		PlainSize: snapshotSize,
		OldFid:    oldUploadedFid,
		Nonce:     n.fileNonce,
		UploadID:  uploadID,
		LastPart:  lastPart,
		DataReader: func() (io.ReadCloser, error) {
			if localPath == "" {
				return nil, fmt.Errorf("staging file path is empty")
			}
			return fs.staging.OpenReader(localPath)
		},
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
	w, wOk := fs.drv.(drive.Writer)

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
			log.L.Infof("syncFilePostUpload: moving %s (fid=%s) from parent %s to %s\n", path, newFid, parentFid, curParentFid)
			if wOk {
				moveEntry := drive.Entry{ID: newFid}
				if err := w.Move(context.Background(), moveEntry, curParentFid); err != nil {
					log.L.Errorf("syncFilePostUpload: move failed for %s: %v\n", path, err)
				} else {
					log.L.Debugf("syncFilePostUpload: move succeeded\n")
				}
			}
		} else if curParentFid != parentFid {
			log.L.Debugf("syncFilePostUpload: parentFid changed but skipping move (local_ prefix): %s -> %s\n", parentFid, curParentFid)
		}

		if curName != snapshotName {
			encName := fs.cipher.EncryptSegment(curName)
			log.L.Infof("syncFilePostUpload: renaming %s from %s to %s\n", path, snapshotName, curName)
			if wOk {
				renameEntry := drive.Entry{ID: newFid}
				if err := w.Rename(context.Background(), renameEntry, encName); err != nil {
					log.L.Errorf("syncFilePostUpload: rename failed for %s: %v\n", path, err)
				}
			}
		}
	}

	if oldFid != "" && oldFid != newFid {
		log.L.Debugf("syncFilePostUpload: replacing fid index %s -> %s for %s\n", oldFid, newFid, path)
		fs.fidNodes.Delete(oldFid)
		if fs.cacheMgr != nil {
			fs.cacheMgr.RemoveChunksByFid(oldFid)
		}
	}
	if newFid != "" && !strings.HasPrefix(newFid, "local_") {
		fs.fidNodes.Store(newFid, n)
	}
}

func (fs *QryptFS) fileExistsOnServerDetailed(fid, parentFid string) (*drive.Entry, error) {
	if fid == "" || strings.HasPrefix(fid, "local_") {
		return nil, nil
	}
	files, err := fs.drv.List(context.Background(), parentFid)
	if err != nil {
		if errors.Is(err, drive.ErrNotFound) || strings.Contains(err.Error(), "404") {
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

func (fs *QryptFS) resolveConflict(path string, n *Node, rf drive.Entry) {
	log.L.Infof("resolveConflict: %s remote fid=%s\n", path, rf.ID)
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	conflictPath := fmt.Sprintf("%s [Local Conflict %s]%s", base, time.Now().Format("20060102_150405"), ext)

	n.mu.Lock()
	newName := filepath.Base(conflictPath)
	if !strings.HasPrefix(n.fid, "local_") {
		n.fid = "local_" + newName + "_" + fmt.Sprint(time.Now().UnixNano())
	}
	n.name = newName
	n.uploadID = ""
	if newNonce, err := fs.cipher.GenerateRandomNonce(); err == nil {
		n.fileNonce = newNonce
		n.hasNonce = true
	}
	n.mu.Unlock()

	fs.replaceNodePath(path, conflictPath, n)
	fs.persistPendingPath(path, conflictPath, n)

	decSize, errDec := fs.cipher.DecryptedSize(rf.Size)
	if errDec != nil {
		log.L.Warnf("resolveConflict: DecryptedSize failed for %s fid=%s encSize=%d: %v\n", path, rf.ID, rf.Size, errDec)
	}
	modTime := rf.ModTime
	fs.storeNode(path, &Node{
		fid:               rf.ID,
		parentFid:         n.parentFid,
		name:              filepath.Base(path),
		size:              decSize,
		encSize:           rf.Size,
		currentPath:       path,
		isFolder:          rf.IsDir,
		mtime:             modTime,
		baseServerMtime:   modTime.UnixMilli(),
		baseServerSize:    decSize,
		lastMetadataCheck: time.Now(),
		source:            "remote",
	})
}

func (fs *QryptFS) ensureParentDirExists(filePath, parentFid string) error {
	if parentFid == "" || parentFid == "0" || parentFid == "root" {
		return nil
	}

	if fs.dirExistsOnServer(parentFid) {
		return nil
	}
	log.L.Infof("ensureParentDirExists: parent not found on server for %s (parentFid=%s)\n", filePath, parentFid)

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

		encName := fs.cipher.EncryptSegment(mountName)
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
		encName := fs.cipher.EncryptSegment(level.segName)
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
	_, err := fs.drv.List(context.Background(), fid)
	if err != nil {
		if errors.Is(err, drive.ErrNotFound) || strings.Contains(err.Error(), "404") {
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
	w, ok := fs.drv.(drive.Writer)
	if !ok {
		return "", fmt.Errorf("driver does not support write operations")
	}
	entry, err := w.Mkdir(context.Background(), parentFid, encName)
	if err == nil {
		return entry.ID, nil
	}
	if errors.Is(err, drive.ErrDirAlreadyExists) {
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
