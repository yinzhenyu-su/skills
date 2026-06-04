//go:build !nofuse

package fusefs

import (
	"context"
	"errors"
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

	const batchSize = 100
	const idleTimeout = 200 * time.Millisecond

	for task := range fs.metadataOpChan {
		tasks := []metadataTask{task}
		if task.opType == "DELETE" {
		collect:
			for len(tasks) < batchSize {
				select {
				case next := <-fs.metadataOpChan:
					if next.opType == "DELETE" {
						tasks = append(tasks, next)
					} else {
						logging.L.Debugf("metadataWorker: processing %d tasks (%d DELETE, then %s)\n", len(tasks)+1, len(tasks), next.opType)
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

func (fs *QryptFS) logOpsBatch(tasks []metadataTask) {
	if fs.cacheMgr == nil {
		return
	}
	for _, t := range tasks {
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
		fs.cacheMgr.MarkOpsLogDone(t.path)
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
	fs.logOpsBatch(tasks)

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
	if validTasks[0].opType == "DELETE" {
		for _, t := range validTasks {
			for _, fid := range t.fids {
				if fid == "" || strings.HasPrefix(fid, "local_") {
					continue
				}
				deleteFids = append(deleteFids, fid)
			}
		}
	}

	if validTasks[0].opType == "LOCAL_CLEANUP" || validTasks[0].opType == "LOCAL_CLEANUP_DIR" {
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
	fs.markOpsDone(tasks)
	go fs.asyncDelete(deleteFids, finalFids, finalPaths, validTasks)
}

func (fs *QryptFS) replayOpsLog() {
	if fs.cacheMgr == nil {
		return
	}
	entries, err := fs.cacheMgr.LoadOpsLog()
	if err != nil || len(entries) == 0 {
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
	default:
		logging.L.Warnf("metadataWorker: unknown opType=%s\n", task.opType)
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
		var lastErr error
		for _, fid := range deleteFids {
			entry := drivers.Entry{ID: fid}
			if e := w.Remove(context.Background(), entry); e != nil {
				if !errors.Is(e, drivers.ErrNotFound) {
					lastErr = e
					break
				}
			}
		}
		err = lastErr

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
		for _, fid := range deleteFids {
			if val, ok := fs.activeDeletions.Load(fid); ok {
				state := val.(*deletionState)
				state.apiDone = true
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
	}
	if err != nil {
		logging.L.Errorf("asyncDelete: FAILED after 3 attempts for %d FIDs (took %v): %v\n", len(deleteFids), time.Since(start), err)
	}
}
