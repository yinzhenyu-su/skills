package fs

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/log"
	"github.com/yinzhenyu/skills/qrypt/internal/quark"
)

func (fs *QryptFS) uploadWorker() {
	defer fs.workerWg.Done()
	defer func() {
		if r := recover(); r != nil {
			log.L.Errorf("PANIC in uploadWorker: %v\n", r)
		}
	}()
	log.L.Info("Upload worker started\n")

	for task := range fs.uploadChan {
		savedPath := task.node.currentPath
		err := fs.syncFile(task.node.currentPath, task.node)

		if err != nil {
			log.L.Warnf("uploadWorker: syncFile failed for %s: %v\n", task.node.currentPath, err)
			retryCount := fs.incrementRetryCount(task.node)
			if retryCount < fs.maxRetries {
				backoff := time.Duration(2<<uint(retryCount-1)) * time.Second
				log.L.Warnf("uploadWorker: will retry %s in %v (retry %d/%d)\n", task.node.currentPath, backoff, retryCount+1, fs.maxRetries)

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
					select {
					case fs.uploadChan <- syncTask{node: n}:
					default:
						if atomic.LoadInt32(&fs.shuttingDown) == 1 {
							n.mu.Lock()
							n.syncQueued = false
							n.mu.Unlock()
							return
						}
						n.mu.Lock()
						n.syncQueued = false
						n.mu.Unlock()
					}
				}(task.node, backoff)
			} else {
				task.node.mu.Lock()
				task.node.isDirty = false
				task.node.syncQueued = false
				task.node.mu.Unlock()
				if fs.cacheMgr != nil {
					fs.cacheMgr.RemovePendingNode(task.node.currentPath)
				}
				fs.retryState.Delete(task.node)
			}
		} else {
			log.L.Infof("Sync: upload succeeded for %s\n", task.node.currentPath)
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
			log.L.Errorf("PANIC in metadataWorker: %v\n", r)
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
						log.L.Debugf("metadataWorker: processing %d tasks (%d DELETE, then %s)\n", len(tasks)+1, len(tasks), next.opType)
						fs.processBatchMetadataTasks(tasks)
						tasks = []metadataTask{next}
						break collect
					}
				case <-time.After(idleTimeout):
					break collect
				}
			}
		}
		log.L.Debugf("metadataWorker: processing batch of %d %s tasks\n", len(tasks), tasks[0].opType)
		fs.processBatchMetadataTasks(tasks)
	}
}

func (fs *QryptFS) processBatchMetadataTasks(tasks []metadataTask) {
	defer func() {
		if r := recover(); r != nil {
			log.L.Errorf("PANIC in processBatchMetadataTasks: %v\n", r)
		}
	}()
	if len(tasks) == 0 {
		return
	}
	start := time.Now()
	log.L.Debugf("processBatchMetadataTasks: start op=%s count=%d\n", tasks[0].opType, len(tasks))

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
					log.L.Debugf("processBatchMetadataTasks: DELETE for %s skipped (tombstone gone)\n", t.path)
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
		log.L.Debugf("processBatchMetadataTasks: all %d tasks invalid after filtering (took %v)\n", len(tasks), time.Since(start))
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
		log.L.Debugf("processBatchMetadataTasks: local cleanup for %d tasks\n", len(validTasks))
		if fs.cacheMgr != nil {
			fs.cacheMgr.BatchDeleteNodeState(finalFids, finalPaths)
		}
		if fs.staging != nil {
			for _, t := range validTasks {
				if t.node != nil && t.node.localPath != "" {
					fs.staging.Remove(t.node.localPath)
				}
			}
		}
		log.L.Debugf("processBatchMetadataTasks: local cleanup done (took %v)\n", time.Since(start))
		return
	}

	if len(deleteFids) == 0 {
		log.L.Debugf("processBatchMetadataTasks: no remote FIDs to delete, local cleanup only\n")
		if fs.cacheMgr != nil {
			fs.cacheMgr.BatchDeleteNodeState(finalFids, finalPaths)
		}
		log.L.Debugf("processBatchMetadataTasks: done (took %v)\n", time.Since(start))
		return
	}

	log.L.Infof("processBatchMetadataTasks: queue %d FIDs for async delete (%d valid tasks, took %v)\n",
		len(deleteFids), len(validTasks), time.Since(start))
	go fs.asyncDelete(deleteFids, finalFids, finalPaths, validTasks)
}

func (fs *QryptFS) asyncDelete(deleteFids, finalFids, finalPaths []string, validTasks []metadataTask) {
	start := time.Now()
	log.L.Infof("asyncDelete: start deleting %d FIDs\n", len(deleteFids))

	defer func() {
		if r := recover(); r != nil {
			log.L.Errorf("PANIC in asyncDelete: %v\n", r)
		}
	}()

	var err error
	for attempt := 0; attempt < 3; attempt++ {
		done := make(chan error, 1)
		go func() {
			done <- fs.manageSvc.Delete(deleteFids)
		}()
		select {
		case err = <-done:
			if err != nil {
				log.L.Warnf("asyncDelete: attempt %d/3 returned error: %v\n", attempt+1, err)
			}
		case <-time.After(30 * time.Second):
			err = fmt.Errorf("DELETE API timeout after 30s (attempt %d/3)", attempt+1)
			log.L.Warnf("asyncDelete: timeout for %d FIDs, attempt %d\n", len(deleteFids), attempt+1)
		}
		if err != nil {
			msg := strings.ToLower(err.Error())
			if strings.Contains(msg, "404") || strings.Contains(msg, "not found") ||
				strings.Contains(msg, strings.ToLower(quark.ErrFileNotFound)) ||
				strings.Contains(msg, strings.ToLower(quark.ErrAlreadyDeleted)) {
				log.L.Infof("asyncDelete: resource already deleted (404), treating as success\n")
				err = nil
				break
			}
			if attempt < 2 {
				log.L.Warnf("asyncDelete: retry %d/3 in %.0fs: %v\n", attempt+1, float64(attempt+1)*0.5, err)
				time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
			}
		} else {
			break
		}
	}

	if err == nil {
		log.L.Infof("asyncDelete: API success for %d FIDs (took %v)\n", len(deleteFids), time.Since(start))
		for _, fid := range deleteFids {
			if val, ok := fs.activeDeletions.Load(fid); ok {
				state := val.(*deletionState)
				state.apiDone = true
				log.L.Debugf("asyncDelete: tombstone apiDone=true for fid=%s\n", fid)
			}
			if fs.cacheMgr != nil {
				fs.cacheMgr.RemoveChunksByFid(fid)
			}
		}

		if fs.cacheMgr != nil {
			fs.cacheMgr.BatchDeleteNodeState(finalFids, finalPaths)
			log.L.Debugf("asyncDelete: state cleaned up for %d FIDs\n", len(finalFids))
		}

		if fs.staging != nil {
			for _, t := range validTasks {
				if t.node != nil && t.node.localPath != "" {
					fs.staging.Remove(t.node.localPath)
				}
			}
		}
	}
	if err != nil {
		log.L.Errorf("asyncDelete: FAILED after 3 attempts for %d FIDs (took %v): %v\n", len(deleteFids), time.Since(start), err)
	}
}
