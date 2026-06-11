//go:build !nofuse

package fusefs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
)

func (fs *QryptFS) uploadWorker() {
	defer fs.workerWg.Done()
	defer func() {
		if r := recover(); r != nil {
			logging.L.Errorf("PANIC in uploadWorker: %v\n", r)
		}
	}()
	logging.L.Infof("Upload worker started\n")

	for task := range fs.uploadChan {
		savedPath := task.node.currentPath
		err := fs.syncFile(task.node.currentPath, task.node)

		if err != nil {
			logging.L.Warnf("uploadWorker: syncFile failed for %s: %v\n", task.node.currentPath, err)
			retryCount := fs.incrementRetryCount(task.node)
			if retryCount < fs.maxRetries {
				backoff := time.Duration(2<<uint(retryCount-1)) * time.Second
				logging.L.Warnf("uploadWorker: will retry %s in %v (retry %d/%d)\n", task.node.currentPath, backoff, retryCount+1, fs.maxRetries)

				go func(n *Node, d time.Duration) {
					defer func() {
						if r := recover(); r != nil {
							recover()
						}
						n.mu.Lock()
						n.syncQueued = false
						n.mu.Unlock()
					}()
					time.Sleep(d)
					if atomic.LoadInt32(&fs.shuttingDown) == 1 {
						return
					}
					n.mu.RLock()
					cp := n.currentPath
					cancelled := n.IsCancelled()
					n.mu.RUnlock()
					if cp == "" || cancelled {
						return
					}
					n.mu.Lock()
					n.syncQueued = true
					n.mu.Unlock()
					fs.enqueueNode(n)
				}(task.node, backoff)
			} else {
				task.node.mu.Lock()
				task.node.syncQueued = false
				task.node.mu.Unlock()
				logging.L.Warnf("uploadWorker: sync permanently failed for %s, data preserved as dirty\n", task.node.currentPath)
				if fs.cacheMgr != nil {
					fs.cacheMgr.RemovePendingNode(task.node.currentPath)
				}
				fs.retryState.Delete(task.node)
			}
		} else {
			logging.L.Infof("Sync: upload succeeded for %s\n", task.node.currentPath)
			fs.resetRetryCount(task.node)

			task.node.mu.RLock()
			newFid := task.node.fid
			task.node.mu.RUnlock()

			if savedPath != "" && !strings.HasPrefix(newFid, "local_") {
				stillInTree := false
				if v, ok := fs.nodes.Load(savedPath); ok && v.(*Node) == task.node {
					stillInTree = true
				}
				if !stillInTree {
					if atomic.LoadInt32(&fs.shuttingDown) == 0 {
						fs.metadataOpChan <- metadataTask{
							opType: "DELETE",
							path:   savedPath,
							fids:   []string{newFid},
						}
					}
				}
			}

			if fs.cacheMgr != nil {
				fs.cacheMgr.RemovePendingNode(savedPath)
			}
		}
	}
}

func (fs *QryptFS) getRetryCount(n *Node) int {
	if v, ok := fs.retryState.Load(n); ok {
		return v.(int)
	}
	return 0
}

func (fs *QryptFS) incrementRetryCount(n *Node) int {
	count := fs.getRetryCount(n) + 1
	fs.retryState.Store(n, count)
	return count
}

func (fs *QryptFS) resetRetryCount(n *Node) {
	fs.retryState.Delete(n)
}

func (fs *QryptFS) metadataWorker() {
	defer fs.workerWg.Done()
	defer func() {
		if r := recover(); r != nil {
			logging.L.Errorf("PANIC in metadataWorker: %v\n", r)
		}
	}()

	const batchSize = 4096
	const idleTimeout = 600 * time.Millisecond

	for task := range fs.metadataOpChan {
		tasks := []metadataTask{task}
		if isDeleteFamilyOp(task.opType) {
		collect:
			for len(tasks) < batchSize {
				select {
				case next, ok := <-fs.metadataOpChan:
					if !ok {
						break collect
					}
					if isDeleteFamilyOp(next.opType) {
						tasks = append(tasks, next)
					} else {
						logging.L.Debugf("metadataWorker: processing %d delete-family tasks before %s\n", len(tasks), next.opType)
						fs.processBatchMetadataTasks(tasks)
						tasks = []metadataTask{next}
						break collect
					}
				case <-time.After(idleTimeout):
					break collect
				}
			}
		}
		logging.L.Debugf("metadataWorker: processing batch of %d %s tasks\n", len(tasks), tasks[0].opType)
		fs.processBatchMetadataTasks(tasks)
	}
}

func isDeleteFamilyOp(opType string) bool {
	return opType == "DELETE" || opType == "LOCAL_CLEANUP" || opType == "LOCAL_CLEANUP_DIR"
}

func taskDeletesDir(t metadataTask) bool {
	if t.opType != "DELETE" {
		return false
	}
	if t.node == nil {
		return false
	}
	t.node.mu.RLock()
	isFolder := t.node.isFolder
	t.node.mu.RUnlock()
	return isFolder
}

func pathUnder(parent, child string) bool {
	if parent == "" || parent == "/" || child == "" || parent == child {
		return false
	}
	if !strings.HasSuffix(parent, "/") {
		parent += "/"
	}
	return strings.HasPrefix(child, parent)
}

func coalesceDeleteFamily(tasks []metadataTask) []metadataTask {
	if len(tasks) == 0 {
		return tasks
	}

	dirDeletes := make([]metadataTask, 0)
	for _, t := range tasks {
		if taskDeletesDir(t) {
			dirDeletes = append(dirDeletes, t)
		}
	}
	if len(dirDeletes) == 0 {
		return tasks
	}

	out := make([]metadataTask, len(tasks))
	copy(out, tasks)
	var skipped int
	for i := range out {
		if out[i].opType != "DELETE" || out[i].skipRemote {
			continue
		}
		for _, dirTask := range dirDeletes {
			if pathUnder(dirTask.path, out[i].path) {
				out[i].skipRemote = true
				skipped++
				break
			}
		}
	}
	if skipped > 0 {
		logging.L.Infof("metadataWorker: coalesced %d child delete tasks under directory deletes\n", skipped)
	}
	return out
}

func (fs *QryptFS) logOpsBatch(tasks []metadataTask) {
	if fs.cacheMgr == nil {
		return
	}
	for _, t := range tasks {
		// LOCAL_CLEANUP / LOCAL_CLEANUP_DIR are local-only and do not need
		// crash recovery. Journaling them would produce done-marker mismatches
		// because AppendOpsLog always writes JOpDelete (OpType becomes "DELETE"
		// on recovery), while markOpsDone uses the original op type.
		if t.opType == "LOCAL_CLEANUP" || t.opType == "LOCAL_CLEANUP_DIR" {
			continue
		}
		fs.cacheMgr.AppendOpsLog(&qrypt.OpsLogEntry{
			OpType: t.opType,
			Path:   t.path,
			Fid:    strings.Join(t.fids, ","),
		})
	}
}

func (fs *QryptFS) markOpsDone(tasks []metadataTask) {
	if fs.cacheMgr == nil {
		return
	}
	for _, t := range tasks {
		fs.cacheMgr.MarkOpsLogDone(t.path, t.opType, strings.Join(t.fids, ","))
	}
}

func (fs *QryptFS) processBatchMetadataTasks(tasks []metadataTask) {
	defer func() {
		if r := recover(); r != nil {
			logging.L.Errorf("PANIC in processBatchMetadataTasks: %v\n", r)
		}
	}()
	if len(tasks) == 0 {
		return
	}
	start := time.Now()
	logging.L.Debugf("processBatchMetadataTasks: start op=%s count=%d\n", tasks[0].opType, len(tasks))

	// Log ALL tasks first (including coalesced ones) before executing any,
	// so crash recovery has a complete record of pending operations.
	fs.logOpsBatch(tasks)

	// Handle coalesced task types (COALESCED_RENAME, COALESCED_MKDIR) after
	// logging, so they survive a crash mid-execution. Collect successful ones
	// to mark done; failed ones stay pending for replay.
	remainingTasks := make([]metadataTask, 0, len(tasks))
	var coalescedDone []metadataTask
	for _, t := range tasks {
		switch t.opType {
		case "COALESCED_RENAME":
			if err := fs.executeCoalescedRename(t); err != nil {
				logging.L.Warnf("processBatchMetadataTasks: coalesced rename %s failed (will retry): %v\n", t.path, err)
			} else {
				coalescedDone = append(coalescedDone, t)
			}
		case "COALESCED_MKDIR":
			if err := fs.executeCoalescedMkdir(t); err != nil {
				logging.L.Warnf("processBatchMetadataTasks: coalesced mkdir %s failed (will retry): %v\n", t.path, err)
			} else {
				coalescedDone = append(coalescedDone, t)
			}
		default:
			remainingTasks = append(remainingTasks, t)
		}
	}
	if len(coalescedDone) > 0 {
		fs.markOpsDone(coalescedDone)
	}
	if len(remainingTasks) == 0 {
		logging.L.Debugf("processBatchMetadataTasks: all tasks were coalesced operations (took %v)\n", time.Since(start))
		return
	}
	tasks = remainingTasks

	tasks = coalesceDeleteFamily(tasks)

	validTasks := make([]metadataTask, 0, len(tasks))
	var finalFids []string
	var finalPaths []string

	for _, t := range tasks {
		if t.opType == "DELETE" {
			stillValid := false
			for _, fid := range t.fids {
				if _, exists := fs.activeDeletions.Load(fid); exists {
					stillValid = true
				} else {
					logging.L.Debugf("processBatchMetadataTasks: DELETE for %s skipped (tombstone gone)\n", t.path)
				}
			}
			if !stillValid {
				continue
			}
		}
		validTasks = append(validTasks, t)
		if t.node != nil {
			finalFids = append(finalFids, t.node.fid)
			finalPaths = append(finalPaths, t.path)
		}
	}

	if len(validTasks) == 0 {
		logging.L.Debugf("processBatchMetadataTasks: all %d tasks invalid after filtering (took %v)\n", len(tasks), time.Since(start))
		return
	}

	var deleteFids []string
	for _, t := range validTasks {
		if t.opType != "DELETE" || t.skipRemote {
			continue
		}
		for _, fid := range t.fids {
			if fid == "" || strings.HasPrefix(fid, "local_") {
				continue
			}
			deleteFids = append(deleteFids, fid)
		}
	}

	if len(deleteFids) == 0 && containsOnlyLocalCleanup(validTasks) {
		logging.L.Debugf("processBatchMetadataTasks: local cleanup for %d tasks\n", len(validTasks))
		// Remove staging files FIRST, then clean up pending-node memory.
		// This ordering is critical for crash recovery with pending.journal:
		// if we crash after BatchDeleteNodeState but before staging.Remove,
		// the journal would still have a dirty entry whose staging file
		// still exists → crash recovery would re-upload a deleted file.
		if fs.staging != nil {
			for _, t := range validTasks {
				if t.node != nil && t.node.localPath != "" {
					fs.staging.Remove(t.node.localPath)
				}
			}
		}
		if fs.cacheMgr != nil {
			fs.cacheMgr.BatchDeleteNodeState(finalFids, finalPaths)
		}
		fs.markOpsDone(tasks)
		logging.L.Debugf("processBatchMetadataTasks: local cleanup done (took %v)\n", time.Since(start))
		return
	}

	if len(deleteFids) == 0 {
		logging.L.Debugf("processBatchMetadataTasks: no remote FIDs to delete, local cleanup only\n")
		if fs.cacheMgr != nil {
			fs.cacheMgr.BatchDeleteNodeState(finalFids, finalPaths)
		}
		fs.markOpsDone(tasks)
		logging.L.Debugf("processBatchMetadataTasks: done (took %v)\n", time.Since(start))
		return
	}

	logging.L.Infof("processBatchMetadataTasks: queue %d FIDs for async delete (%d valid tasks, took %v)\n",
		len(deleteFids), len(validTasks), time.Since(start))
	go fs.asyncDelete(deleteFids, finalFids, finalPaths, validTasks)
}

func containsOnlyLocalCleanup(tasks []metadataTask) bool {
	for _, t := range tasks {
		if t.opType != "LOCAL_CLEANUP" && t.opType != "LOCAL_CLEANUP_DIR" {
			return false
		}
	}
	return true
}

func (fs *QryptFS) replayOpsLog() {
	if fs.cacheMgr == nil {
		return
	}
	entries, err := fs.cacheMgr.LoadOpsLog()
	if err != nil {
		logging.L.Warnf("replayOpsLog: failed to load ops log: %v\n", err)
		return
	}
	if len(entries) == 0 {
		logging.L.Debugf("replayOpsLog: no pending operations to replay\n")
		return
	}
	var pending int
	for _, e := range entries {
		if !e.Done {
			pending++
			logging.L.Infof("replayOpsLog: replaying pending %s operation on %s\n", e.OpType, e.Path)
			fs.processMetadataTask(metadataTask{
				opType: e.OpType,
				path:   e.Path,
				fids:   strings.Split(e.Fid, ","),
			})
		}
	}
	if pending > 0 {
		logging.L.Infof("replayOpsLog: replayed %d pending operations\n", pending)
	}
	fs.cacheMgr.PurgeOpsLog(72 * time.Hour)
}

func (fs *QryptFS) processMetadataTask(task metadataTask) {
	switch task.opType {
	case "DELETE":
		fs.asyncDelete(task.fids, nil, nil, []metadataTask{task})
	case "COALESCED_RENAME":
		if err := fs.executeCoalescedRename(task); err != nil {
			logging.L.Warnf("processMetadataTask: coalesced rename %s failed (will retry): %v\n", task.path, err)
		} else {
			fs.markOpsDone([]metadataTask{task})
		}
	case "COALESCED_MKDIR":
		if err := fs.executeCoalescedMkdir(task); err != nil {
			logging.L.Warnf("processMetadataTask: coalesced mkdir %s failed (will retry): %v\n", task.path, err)
		} else {
			fs.markOpsDone([]metadataTask{task})
		}
	default:
		logging.L.Warnf("metadataWorker: unknown opType=%s\n", task.opType)
	}
}

// handleCoalescedIntents is the flush callback for OperationCoalescer. It receives
// coalesced RemoteIntent values and dispatches them to the appropriate processing
// paths (metadataOpChan, upload, or local cleanup).
func (fs *QryptFS) handleCoalescedIntents(intents []RemoteIntent) {
	for _, intent := range intents {
		if intent.SkipRemote {
			// Local cleanup only - remove staging
			if intent.Staging != "" && fs.staging != nil {
				fs.staging.Remove(intent.Staging)
			}
			if intent.Node != nil && intent.Node.localPath != "" && fs.staging != nil {
				fs.staging.Remove(intent.Node.localPath)
			}
			if fs.cacheMgr != nil && intent.Fid != "" {
				fs.cacheMgr.RemovePendingNode(intent.Path)
			}
			continue
		}
		switch intent.Kind {
		case "UPLOAD":
			if intent.Node != nil {
				intent.Node.mu.Lock()
				intent.Node.syncQueued = true
				intent.Node.mu.Unlock()
				fs.enqueueNode(intent.Node)
			}
		case "DELETE":
			// Route to existing metadata worker
			fids := intent.Fids
			if len(fids) == 0 && intent.Fid != "" {
				fids = []string{intent.Fid}
			}
			// Only submit to metadataOpChan if there are remote fids
			hasRemoteFid := false
			for _, fid := range fids {
				if fid != "" && !strings.HasPrefix(fid, "local_") {
					hasRemoteFid = true
					break
				}
			}
			if hasRemoteFid {
				fs.metadataOpChan <- metadataTask{
					opType: "DELETE",
					path:   intent.Path,
					node:   intent.Node,
					fids:   fids,
				}
			} else if intent.Node != nil {
				// Local-only delete
				fs.metadataOpChan <- metadataTask{
					opType: "LOCAL_CLEANUP",
					path:   intent.Path,
					node:   intent.Node,
				}
			}
		case "RENAME":
			// Deferred rename - store newPath on both the node and the task's fids.
			// fids[0]=fid, fids[1]=newPath (for crash recovery replay without Node).
			if intent.Node != nil {
				intent.Node.mu.Lock()
				intent.Node.expectedFid = intent.NewPath
				intent.Node.mu.Unlock()
			}
			fs.metadataOpChan <- metadataTask{
				opType: "COALESCED_RENAME",
				path:   intent.Path,
				node:   intent.Node,
				fids:   []string{intent.Fid, intent.NewPath},
			}
		case "MKDIR":
			// Deferred mkdir
			fs.metadataOpChan <- metadataTask{
				opType: "COALESCED_MKDIR",
				path:   intent.Path,
				node:   intent.Node,
			}
		}
	}
}

func (fs *QryptFS) lruEvictionLoop() {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			fs.evictStaleNodes()
		case <-fs.lruStop:
			return
		}
	}
}

func (fs *QryptFS) evictStaleNodes() {
	const maxNodeAge = 30 * time.Minute
	const maxNodes = 50000

	var count int
	fs.nodes.Range(func(key, value interface{}) bool {
		count++
		if count <= 1000 {
			return true
		}
		n := value.(*Node)
		if n.currentPath == "/" || n.isDirty {
			return true
		}
		n.mu.RLock()
		lastCheck := n.lastMetadataCheck
		n.mu.RUnlock()
		if time.Since(lastCheck) > maxNodeAge {
			n.mu.RLock()
			fid := n.fid
			isDir := n.isFolder
			n.mu.RUnlock()
			if !isDir || (isDir && n.isChildrenEmpty()) {
				fs.deleteNodePath(n.currentPath, n)
				if fid != "" && !strings.HasPrefix(fid, "local_") {
					fs.fidNodes.Delete(fid)
				}
			}
		}
		return true
	})
	if count > maxNodes {
		logging.L.Warnf("lruEviction: node count %d exceeds limit %d, triggering aggressive eviction\n", count, maxNodes)
	}
}

// removeFids deletes the given fids from the driver, using BatchRemove when
// available to avoid N sequential API calls for bulk operations.
func (fs *QryptFS) removeFids(w drivers.Writer, fids []string) error {
	if br, ok := w.(drivers.BatchRemover); ok && len(fids) > 0 {
		entries := make([]drivers.Entry, len(fids))
		for i, fid := range fids {
			entries[i] = drivers.Entry{ID: fid}
		}
		if err := br.BatchRemove(context.Background(), entries); err != nil {
			if errors.Is(err, drivers.ErrNotFound) {
				return nil
			}
			return err
		}
		return nil
	}
	// Fallback: individual removes for drivers without batch support.
	for _, fid := range fids {
		entry := drivers.Entry{ID: fid}
		if err := w.Remove(context.Background(), entry); err != nil {
			if !errors.Is(err, drivers.ErrNotFound) {
				return err
			}
		}
	}
	return nil
}

func (fs *QryptFS) asyncDelete(deleteFids, finalFids, finalPaths []string, validTasks []metadataTask) {
	start := time.Now()
	logging.L.Infof("asyncDelete: start deleting %d FIDs\n", len(deleteFids))

	defer func() {
		if r := recover(); r != nil {
			logging.L.Errorf("PANIC in asyncDelete: %v\n", r)
		}
	}()

	w, ok := fs.drv.(drivers.Writer)
	if !ok {
		logging.L.Errorf("asyncDelete: driver does not support delete\n")
		return
	}

	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = fs.removeFids(w, deleteFids)

		if err != nil {
			logging.L.Warnf("asyncDelete: attempt %d/3 returned error: %v\n", attempt+1, err)
			if attempt < 2 {
				logging.L.Warnf("asyncDelete: retry %d/3 in %.0fs: %v\n", attempt+1, float64(attempt+1)*0.5, err)
				time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
			}
		} else {
			break
		}
	}

	if err == nil {
		logging.L.Infof("asyncDelete: API success for %d FIDs (took %v)\n", len(deleteFids), time.Since(start))
		for _, fid := range completedDeleteFids(deleteFids, validTasks) {
			if val, ok := fs.activeDeletions.Load(fid); ok {
				state := val.(*deletionState)
				state.apiDone = true
				if state.path != "" {
					fs.markDeletingPathDone(state.path)
				}
				logging.L.Debugf("asyncDelete: tombstone apiDone=true for fid=%s\n", fid)
			}
			if fs.cacheMgr != nil {
				fs.cacheMgr.RemoveChunksByFid(fid)
			}
		}

		// Remove staging files before deleting pending-node state.
		// See LOCAL_CLEANUP above for crash-recovery ordering rationale.
		if fs.staging != nil {
			for _, t := range validTasks {
				if t.node != nil && t.node.localPath != "" {
					fs.staging.Remove(t.node.localPath)
				}
			}
		}
		if fs.cacheMgr != nil {
			fs.cacheMgr.BatchDeleteNodeState(finalFids, finalPaths)
			logging.L.Debugf("asyncDelete: state cleaned up for %d FIDs\n", len(finalFids))
		}
		// Mark journal entries as done so successful deletes are not replayed
		// on restart (replayOpsLog path). Filter to only DELETE-type tasks
		// (LOCAL_CLEANUP is not journaled, coalesced ops are marked in processBatchMetadataTasks).
		var deleteTasks []metadataTask
		for _, t := range validTasks {
			if t.opType == "DELETE" {
				deleteTasks = append(deleteTasks, t)
			}
		}
		if len(deleteTasks) > 0 {
			fs.markOpsDone(deleteTasks)
		}
	}
	if err != nil {
		logging.L.Errorf("asyncDelete: FAILED after 3 attempts for %d FIDs (took %v): %v\n", len(deleteFids), time.Since(start), err)
	}
}

func completedDeleteFids(deleteFids []string, tasks []metadataTask) []string {
	seen := make(map[string]bool)
	var fids []string
	for _, fid := range deleteFids {
		if fid == "" || seen[fid] {
			continue
		}
		seen[fid] = true
		fids = append(fids, fid)
	}
	for _, t := range tasks {
		if t.opType != "DELETE" {
			continue
		}
		for _, fid := range t.fids {
			if fid == "" || strings.HasPrefix(fid, "local_") || seen[fid] {
				continue
			}
			seen[fid] = true
			fids = append(fids, fid)
		}
	}
	return fids
}

// executeCoalescedRename performs a deferred remote rename for a COALESCED_RENAME
// metadataTask. The new path is read from node.expectedFid (live path) or from
// t.fids[1] (crash recovery path encoded by handleCoalescedIntents).
func (fs *QryptFS) executeCoalescedRename(t metadataTask) error {
	w, wOk := fs.drv.(drivers.Writer)
	if !wOk {
		return fmt.Errorf("driver does not support rename")
	}

	var fid, parentFid, newPath string
	if t.node != nil {
		t.node.mu.RLock()
		fid = t.node.fid
		parentFid = t.node.parentFid
		newPath = t.node.expectedFid
		t.node.mu.RUnlock()
	}

	// Crash recovery fallback: fid and newPath encoded in t.fids
	if fid == "" && len(t.fids) > 0 {
		fid = t.fids[0]
	}
	if newPath == "" && len(t.fids) > 1 {
		newPath = t.fids[1]
	}

	if fid == "" || strings.HasPrefix(fid, "local_") {
		return nil // nothing to do — local-only file
	}
	if newPath == "" {
		return fmt.Errorf("no newPath for %s", t.path)
	}

	oldPath := t.path
	oldName := filepath.Base(oldPath)
	newName := filepath.Base(newPath)

	encName := fs.cp.EncryptSegment(newName)
	oldEncName := fs.cp.EncryptSegment(oldName)
	var lastErr error
	if encName != oldEncName {
		renameEntry := drivers.Entry{ID: fid, ParentID: parentFid}
		for attempt := 0; attempt < 5; attempt++ {
			if err := w.Rename(context.Background(), renameEntry, encName); err != nil {
				if errors.Is(err, drivers.ErrDirAlreadyExists) || strings.Contains(err.Error(), "conflict") {
					time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
					continue
				}
				lastErr = fmt.Errorf("rename failed for %s: %w", oldPath, err)
				logging.L.Errorf("executeCoalescedRename: %v\n", lastErr)
				return lastErr
			}
			break
		}
	}

	oldParent := filepath.Dir(oldPath)
	newParent := filepath.Dir(newPath)
	if oldParent != newParent {
		newParentNode, errc := fs.lookup(newParent)
		if errc == 0 {
			moveEntry := drivers.Entry{ID: fid, ParentID: parentFid}
			for attempt := 0; attempt < 5; attempt++ {
				if err := w.Move(context.Background(), moveEntry, newParentNode.fid); err != nil {
					if errors.Is(err, drivers.ErrDirAlreadyExists) || strings.Contains(err.Error(), "conflict") {
						time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
						continue
					}
					lastErr = fmt.Errorf("move failed for %s: %w", oldPath, err)
					logging.L.Errorf("executeCoalescedRename: %v\n", lastErr)
					return lastErr
				}
				break
			}
		}
	}
	return nil
}

// executeCoalescedMkdir performs a deferred remote Mkdir for a COALESCED_MKDIR
// metadataTask. On success, it updates the local Node's fid to the remote fid
// so subsequent coalesced deletes/renames see RemoteExists=true.
func (fs *QryptFS) executeCoalescedMkdir(t metadataTask) error {
	w, wOk := fs.drv.(drivers.Writer)
	if !wOk {
		return fmt.Errorf("driver does not support mkdir")
	}
	path := t.path
	parentPath := filepath.Dir(path)
	name := filepath.Base(path)

	parentNode, errc := fs.lookup(parentPath)
	if errc != 0 {
		return fmt.Errorf("parent lookup failed: errc=%d", errc)
	}

	encName := fs.cp.EncryptSegment(name)
	entry, err := w.Mkdir(context.Background(), parentNode.fid, encName)
	if err != nil {
		// Crash recovery: the directory may already exist from a previous
		// attempt. Treat this as success and look up the existing entry.
		if errors.Is(err, drivers.ErrDirAlreadyExists) {
			existingFid, lookupErr := fs.findChildDir(context.Background(), parentNode.fid, encName)
			if lookupErr != nil {
				return fmt.Errorf("mkdir %s already exists but lookup failed: %w", path, lookupErr)
			}
			entry = drivers.Entry{ID: existingFid, Name: name, IsDir: true}
		} else {
			logging.L.Warnf("executeCoalescedMkdir: mkdir failed for %s: %v\n", path, err)
			return err
		}
	}

	if t.node != nil && entry.ID != "" {
		t.node.mu.Lock()
		t.node.fid = entry.ID
		t.node.source = "remote"
		t.node.mu.Unlock()
		fs.fidNodes.Store(entry.ID, t.node)
		logging.L.Debugf("executeCoalescedMkdir: updated %s fid local→%s\n", path, entry.ID)
	}
	return nil
}
