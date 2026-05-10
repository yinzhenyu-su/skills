package vfs

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync/atomic"
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
	// Check if it exists on server directly — if not, recreate it under Quark root "0".
	if len(levels) == 0 {
		if parentFid == "" || parentFid == "0" || parentFid == "root" {
			return nil
		}
		driver.Log.Infof("ensureParentDirExists: file at mount root, checking parentFid=%s\n", parentFid)
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

		driver.Log.Infof("ensureParentDirExists: mount root dir missing on server, name=%q, oldFid=%s\n", mountName, parentFid)

		if mountName == "" {
			return fmt.Errorf("mount root node has empty name, cannot recreate directory")
		}

		encName := fs.cipher.EncryptSegment(mountName)
		newFid, createErr := fs.driver.CreateDir("0", encName)
		if createErr != nil {
			if strings.Contains(createErr.Error(), driver.QuarkErrDirAlreadyExists) {
				// Dir might already exist — try to find it
				time.Sleep(2 * time.Second)
				fs.driver.RemoveDirCache("0")
				if found, findErr := fs.driver.FindChildByName("0", encName); findErr == nil {
					newFid = found
				} else {
					return createErr
				}
			} else {
				return createErr
			}
		}
		mountRootNode.mu.Lock()
		mountRootNode.fid = newFid
		mountRootNode.mu.Unlock()
		fs.fidNodes.Store(newFid, mountRootNode)
		return nil
	}

	// Walk from root down to parent, finding or creating each directory.
	currentRemoteParentFid := "0" // Start from Quark root
	for _, level := range levels {
		encName := fs.cipher.EncryptSegment(level.segName)
		fid, err := fs.driver.FindChildByName(currentRemoteParentFid, encName)
		if err != nil {
			// Directory missing, create it
			driver.Log.Infof("ensureParentDirExists: creating missing directory %s under %s\n", level.segName, currentRemoteParentFid)
			newFid, createErr := fs.driver.CreateDir(currentRemoteParentFid, encName)
			if createErr != nil {
				if strings.Contains(createErr.Error(), driver.QuarkErrDirAlreadyExists) {
					// Dir might already exist (race condition)
					time.Sleep(2 * time.Second)
					fs.driver.RemoveDirCache(currentRemoteParentFid)
					if found, findErr := fs.driver.FindChildByName(currentRemoteParentFid, encName); findErr == nil {
						newFid = found
					} else {
						return createErr
					}
				} else {
					return createErr
				}
			}
			fid = newFid
		}

		// Update local node if it exists
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
	_, err := fs.driver.ListFiles(fid)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "404") || strings.Contains(msg, "not found") || strings.Contains(msg, driver.QuarkErrFileNotFound) {
			return false
		}
	}
	return true
}

func (fs *QryptFS) fileExistsOnServer(fid, parentFid string) bool {
	rf, _ := fs.fileExistsOnServerDetailed(fid, parentFid)
	return rf != nil
}

func (fs *QryptFS) fileExistsOnServerDetailed(fid, parentFid string) (*driver.File, error) {
	if fid == "" || strings.HasPrefix(fid, "local_") {
		return nil, nil
	}

	files, err := fs.driver.ListFiles(parentFid)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "404") || strings.Contains(msg, "not found") || strings.Contains(msg, driver.QuarkErrFileNotFound) {
			return nil, nil
		}
		return nil, err
	}

	for _, f := range files {
		if f.Fid == fid {
			return &f, nil
		}
	}
	return nil, nil
}

func (fs *QryptFS) uploadWorker() {
	defer func() {
		if r := recover(); r != nil {
			driver.Log.Errorf("PANIC in uploadWorker: %v\n%s\n", r, debug.Stack())
		}
	}()
	driver.Log.Info("Upload worker started\n")
	for task := range fs.uploadChan {
		// 保存上传前的路径，用于上传后检测节点是否已被 Unlink 删除
		savedPath := task.node.currentPath
		err := fs.syncFile(task.node.currentPath, task.node)

		if err != nil {
			driver.Log.Errorf("Sync: failed to sync %s: %v\n", task.node.currentPath, err)

			// VFS 级重试（Layer 3）：当 HTTP 层和分片级重试都耗尽后，在此重新入队
			retryCount := fs.incrementRetryCount(task.node)
			if retryCount < fs.maxRetries {
				backoff := time.Duration(2<<uint(retryCount-1)) * time.Second // 2s, 4s, 8s, 16s, 32s
				driver.Log.Warnf("uploadWorker: retry %d/%d for %s after %v\n",
					retryCount+1, fs.maxRetries, task.node.currentPath, backoff)

				go func(n *node, d time.Duration) {
					defer func() {
						if r := recover(); r != nil {
							driver.Log.Errorf("PANIC in uploadWorker retry goroutine: %v\n%s\n", r, debug.Stack())
							n.mu.Lock()
							n.syncQueued = false
							n.mu.Unlock()
						}
					}()
					time.Sleep(d)
					// Shutdown 保护：shuttingDown 在 Shutdown() 中先于 close(channel) 设置
					if atomic.LoadInt32(&fs.shuttingDown) == 1 {
						n.mu.Lock()
						n.syncQueued = false
						n.mu.Unlock()
						return
					}
					// 检查节点是否仍有效
					n.mu.RLock()
					cp := n.currentPath
					cancelled := n.isCancelled()
					n.mu.RUnlock()
					if cp == "" || cancelled {
						return // 节点已被删除，不重试
					}
					n.mu.Lock()
					n.syncQueued = true
					n.mu.Unlock()
					fs.uploadChan <- syncTask{node: n}
				}(task.node, backoff)
			} else {
				driver.Log.Errorf("uploadWorker: max retries (%d) exhausted for %s, giving up\n", fs.maxRetries, task.node.currentPath)
				// 放弃：清理本地状态
				task.node.mu.Lock()
				task.node.isDirty = false
				task.node.syncQueued = false
				task.node.mu.Unlock()
				if fs.cache != nil {
					_ = fs.cache.RemovePendingNode(task.node.currentPath)
				}
				fs.retryState.Delete(task.node)
			}

			// 更新 opsLog 状态
			if task.opsLogID > 0 && fs.cache != nil {
				if db, ok := fs.cache.GetDB().(*cache.CacheDB); ok {
					_ = db.UpdateOpsLogStatus(task.opsLogID, "FAILED")
				}
			}
		} else {
			// 成功路径：清除重试计数
			fs.resetRetryCount(task.node)

			// 幽灵文件检测：上传成功后，检查节点是否已被 Unlink 删除
			task.node.mu.RLock()
			newFid := task.node.fid
			task.node.mu.RUnlock()

			if savedPath != "" && !strings.HasPrefix(newFid, "local_") {
				stillInTree := false
				if v, ok := fs.nodes.Load(savedPath); ok && v.(*node) == task.node {
					stillInTree = true
				}

				if !stillInTree {
					driver.Log.Infof("uploadWorker: ghost file detected — node at %s was removed during upload (fid=%s), sending DELETE to clean up server\n", savedPath, newFid)
					fs.metadataOpChan <- metadataTask{
						opType: "DELETE",
						path:   savedPath,
						fids:   []string{newFid},
					}
				}
			}

			if task.opsLogID > 0 && fs.cache != nil {
				if db, ok := fs.cache.GetDB().(*cache.CacheDB); ok {
					_ = db.UpdateOpsLogStatus(task.opsLogID, "DONE")
				}
			}
		}
	}
	driver.Log.Info("Upload worker stopped\n")
}

// getRetryCount 返回给定节点的当前重试次数
func (fs *QryptFS) getRetryCount(n *node) int {
	if v, ok := fs.retryState.Load(n); ok {
		return v.(int)
	}
	return 0
}

// incrementRetryCount 原子递增重试计数，返回增加后的值
func (fs *QryptFS) incrementRetryCount(n *node) int {
	count := fs.getRetryCount(n) + 1
	fs.retryState.Store(n, count)
	return count
}

// resetRetryCount 上传成功后清除重试计数
func (fs *QryptFS) resetRetryCount(n *node) {
	fs.retryState.Delete(n)
}

func (fs *QryptFS) recoverDirtyFiles() {
	if fs.cache == nil {
		return
	}
	nodes, err := fs.cache.GetPendingNodes()
	if err != nil {
		driver.Log.Errorf("recoverDirtyFiles: failed to get pending nodes: %v\n", err)
		return
	}

	for _, f := range nodes {
		n, errc := fs.lookupExtended(f.Path, true)
		if errc != 0 {
			// lookupExtended 失败时的兜底处理：

			// Case 1: 已有真实 FID 且 staging 不存在 → 已上传成功，清理残留 DB 记录
			if !strings.HasPrefix(f.Fid, "local_") && f.LocalPath == "" {
				_ = fs.cache.RemovePendingNode(f.Path)
				driver.Log.Infof("recoverDirtyFiles: cleaned up stale record %s (already uploaded, fid=%s)\n", f.Path, f.Fid)
				continue
			}

			// Case 2: local_ FID → 从未上传，尝试从 DB 重建节点树并重新入队
			if strings.HasPrefix(f.Fid, "local_") {
				// 先检查 staging 文件是否存在
				// 如果 staging 已被 crash 前的 Unlink/Rm 操作删除，直接清理 DB 记录
				if f.LocalPath != "" && fs.staging != nil {
					if _, sErr := fs.staging.FileSize(f.LocalPath); sErr != nil {
						_ = fs.cache.RemovePendingNode(f.Path)
						driver.Log.Infof("recoverDirtyFiles: staging missing for %s (deleted before crash), cleaned up\n", f.Path)
						continue
					}
				}
				parentPath := filepath.Dir(f.Path)
				_, parentErr := fs.lookupExtended(parentPath, true)
				if parentErr != 0 {
					driver.Log.Warnf("recoverDirtyFiles: cannot rebuild %s, parent %s not found on server\n", f.Path, parentPath)
					continue
				}

				// 从 DB 记录重建节点
				newNode := &node{
					fid:               f.Fid,
					parentFid:         f.ParentFid,
					name:              f.Name,
					currentPath:       f.Path,
					size:              f.Size,
					isFolder:          f.IsFolder,
					isDirty:           true,
					localPath:         f.LocalPath,
					source:            "local",
					lastMetadataCheck: time.Now(),
					baseServerMtime:   f.BaseServerMtime,
					baseServerSize:    f.BaseServerSize,
					uploadID:          f.UploadID, // 恢复断点续传 ID
				}
				if len(f.Nonce) == 24 {
					copy(newNode.fileNonce[:], f.Nonce)
					newNode.hasNonce = true
				}
				fs.storeNode(f.Path, newNode)
				newNode.mu.Lock()
				newNode.syncQueued = true
				newNode.mu.Unlock()
				fs.uploadChan <- syncTask{node: newNode}
				driver.Log.Infof("recoverDirtyFiles: rebuilt node and queued %s for sync (from DB, fid=%s)\n", f.Path, f.Fid)
				continue
			}

			// Case 3: 未匹配以上规则的记录 → 日志警告，跳过
			driver.Log.Warnf("recoverDirtyFiles: ambiguous state for %s (fid=%s, localPath=%s), skipping\n", f.Path, f.Fid, f.LocalPath)
			continue
		}

		n.mu.Lock()

		// 如果文件已有真实 FID 且 staging 不存在，说明已上传成功，跳过
		if !strings.HasPrefix(n.fid, "local_") && n.localPath == "" {
			n.isDirty = false
			n.syncQueued = false
			n.mu.Unlock()
			_ = fs.cache.RemovePendingNode(f.Path)
			driver.Log.Infof("recoverDirtyFiles: skipped %s (already uploaded, fid=%s)\n", f.Path, n.fid)
			continue
		}

		// staging 文件已被删除（crash 前的 Unlink/Rm 所致）→ 清理 DB 记录，跳过上传
		if n.localPath != "" && fs.staging != nil {
			if _, sErr := fs.staging.FileSize(n.localPath); sErr != nil {
				n.isDirty = false
				n.syncQueued = false
				n.mu.Unlock()
				_ = fs.cache.RemovePendingNode(f.Path)
				driver.Log.Infof("recoverDirtyFiles: staging missing for %s (deleted before crash), cleaned up\n", f.Path)
				continue
			}
		}

		n.isDirty = true
		n.syncQueued = true
		if f.UploadID != "" {
			n.uploadID = f.UploadID // 恢复断点续传 ID
		}
		n.mu.Unlock()
		fs.uploadChan <- syncTask{node: n}
		driver.Log.Infof("recoverDirtyFiles: queued %s for sync\n", f.Path)
	}
}

func (fs *QryptFS) opsLogWorker() {
	defer func() {
		if r := recover(); r != nil {
			driver.Log.Errorf("PANIC in opsLogWorker: %v\n%s\n", r, debug.Stack())
		}
	}()
	const batchSize = 100
	const idleTimeout = 500 * time.Millisecond

	for task := range fs.opsLogChan {

		batch := []metadataTask{task}
	collect:
		for len(batch) < batchSize {
			select {
			case next := <-fs.opsLogChan:
				batch = append(batch, next)
			case <-time.After(idleTimeout):
				break collect
			}
		}

		if fs.cache != nil {
			if db, ok := fs.cache.GetDB().(*cache.CacheDB); ok {
				logData := make([]cache.OpsLogData, 0, len(batch))
				for _, t := range batch {
					// 构造 Payload，同时包含 fids 数组和单个 fid 字段，最大化兼容性
					p := opsPayload{
						Fids: t.fids,
					}
					if len(t.fids) == 1 {
						p.Fid = t.fids[0]
					}
					payload, _ := json.Marshal(p)
					logData = append(logData, cache.OpsLogData{
						OpType:     t.opType,
						SourcePath: t.path,
						Payload:    string(payload),
					})
				}
				err := db.BatchAddOpsLog(logData)
				if err != nil {
					driver.Log.Errorf("opsLogWorker: failed to batch add logs: %v\n", err)
				}
			}
		}
	}
}

func (fs *QryptFS) metadataWorker() {
	defer func() {
		if r := recover(); r != nil {
			driver.Log.Errorf("PANIC in metadataWorker: %v\n%s\n", r, debug.Stack())
		}
	}()
	driver.Log.Info("Metadata worker started\n")
	const batchSize = 100
	const idleTimeout = 200 * time.Millisecond

	for task := range fs.metadataOpChan {
		// 尝试收集一批任务进行批量删除
		tasks := []metadataTask{task}
		if task.opType == "DELETE" {
		collect:
			for len(tasks) < batchSize {
				select {
				case next := <-fs.metadataOpChan:
					if next.opType == "DELETE" {
						tasks = append(tasks, next)
					} else {
						// 类型不同，处理当前已收集的，然后单独处理这个新任务
						fs.processBatchMetadataTasks(tasks)
						tasks = []metadataTask{next}
						break collect
					}
				case <-time.After(idleTimeout):
					break collect
				}
			}
		}
		fs.processBatchMetadataTasks(tasks)
	}
}

func (fs *QryptFS) processBatchMetadataTasks(tasks []metadataTask) {
	if len(tasks) == 0 {
		return
	}

	// 1. 过滤并收集有效任务
	// --- 核心修复：检查墓碑是否还在，如果不在说明已被 Mkdir 撤销 ---
	validTasks := make([]metadataTask, 0, len(tasks))
	var finalFids []string
	var finalPaths []string
	deletingFolders := make(map[string]string) // path -> fid (用于后续合并优化)

	for _, t := range tasks {
		if t.opType == "DELETE" {
			stillValid := false
			for _, fid := range t.fids {
				if _, exists := fs.activeDeletions.Load(fid); exists {
					stillValid = true
				}
			}
			if !stillValid {
				driver.Log.Infof("Metadata worker: skipping %s for %s (tombstone revoked/recreated)\n", t.opType, t.path)
				continue
			}
			// 记录批次中的目录，以便合并子项
			if t.node != nil && t.node.isFolder {
				prefix := t.path
				if !strings.HasSuffix(prefix, "/") {
					prefix += "/"
				}
				if len(t.fids) > 0 {
					deletingFolders[prefix] = t.fids[0]
				}
			}
		}

		validTasks = append(validTasks, t)
		if t.node != nil {
			finalFids = append(finalFids, t.node.fid)
			finalPaths = append(finalPaths, t.path)
		}
	}

	if len(validTasks) == 0 {
		return
	}

	// 1b. 聚合 FID，并根据目录层级进行合并优化（API 层面）
	var deleteFids []string
	if validTasks[0].opType == "DELETE" {
		for _, t := range validTasks {
			for _, fid := range t.fids {
				if fid == "" || strings.HasPrefix(fid, "local_") {
					continue
				}
				// 检查该任务的路径是否在某个待删目录之下（且不是目录本身）
				isRedundant := false
				for folderPath, folderFid := range deletingFolders {
					if strings.HasPrefix(t.path, folderPath) && fid != folderFid {
						isRedundant = true
						break
					}
				}
				if !isRedundant {
					deleteFids = append(deleteFids, fid)
				}
			}
		}
	}

	// 2. 如果是简单的本地清理，直接事务处理
	if validTasks[0].opType == "LOCAL_CLEANUP" || validTasks[0].opType == "LOCAL_CLEANUP_DIR" {
		if fs.cache != nil {
			_ = fs.cache.BatchDeleteNodeState(finalFids, finalPaths)
		}
		if fs.staging != nil {
			for _, t := range validTasks {
				if t.node != nil && t.node.localPath != "" {
					_ = fs.staging.Remove(t.node.localPath)
				}
			}
		}
		return
	}

	// 3. 远端批量删除 API 调用
	if len(deleteFids) == 0 {
		// 可能是被过滤完后的残留，但仍然需要清理本地状态
		if fs.cache != nil {
			_ = fs.cache.BatchDeleteNodeState(finalFids, finalPaths)
		}
		return
	}

	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = fs.driver.Delete(deleteFids)
		if err != nil {
			msg := strings.ToLower(err.Error())
			// 幂等增强：如果网盘返回 404、未找到或已删除，视为删除成功，继续清理本地
			if strings.Contains(msg, "404") || strings.Contains(msg, "not found") ||
				strings.Contains(msg, strings.ToLower(driver.QuarkErrFileNotFound)) ||
				strings.Contains(msg, strings.ToLower(driver.QuarkErrAlreadyDeleted)) {
				err = nil
				break
			}
			driver.Log.Errorf("Metadata worker: batch delete failed (attempt %d/3): %v\n", attempt+1, err)
			time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
		} else {
			break
		}
	}

	// 4. 统一处理状态更新和数据库清理
	if err == nil {
		// 标记墓碑状态并清理物理分块
		for _, fid := range deleteFids {
			if val, ok := fs.activeDeletions.Load(fid); ok {
				state := val.(*deletionState)
				state.apiDone = true
			}
			// 核心改进：物理清理磁盘上的缓存分块
			if fs.cache != nil {
				_ = fs.cache.RemoveChunksByFid(fid)
			}
		}

		if fs.cache != nil {
			// 合并执行数据库删除事务
			_ = fs.cache.BatchDeleteNodeState(finalFids, finalPaths)

			// 批量更新操作日志状态
			if db, ok := fs.cache.GetDB().(*cache.CacheDB); ok {
				// 异步写入日志后，这里无法简单获取 logID，我们通过 path/fid 批量标记
				_ = db.BatchMarkOpsDone(finalFids, finalPaths)
			}
		}
		// 清理 staging
		if fs.staging != nil {
			for _, t := range validTasks {
				if t.node != nil && t.node.localPath != "" {
					_ = fs.staging.Remove(t.node.localPath)
				}
			}
		}

		driver.Log.Infof("Metadata worker: processed batch of %d %s tasks\n", len(validTasks), tasks[0].opType)
	} else {
		driver.Log.Error("Metadata worker: batch FAILED after retries\n")
	}
}

func (fs *QryptFS) syncFile(path string, n *node) (err error) {
	if fs.isUnderDeletingDir(path) {
		driver.Log.Infof("syncFile: aborting sync for %s (being deleted)\n", path)
		n.mu.Lock()
		n.syncQueued = false
		n.mu.Unlock()
		return nil
	}

	// 原子标记检查：节点已被 Unlink 删除，跳过上传
	if n.isCancelled() {
		driver.Log.Infof("syncFile: aborting sync for %s (node cancelled / deleted)\n", path)
		n.mu.Lock()
		n.syncQueued = false
		n.mu.Unlock()
		return nil
	}

	startedAt := time.Now()
	stats := syncPerformanceSnapshot{Path: path}

	// 核心加固：在进入上传前，再次确认节点是否依然处于"活跃"状态（未被删除）
	n.mu.RLock()
	currentPath := n.currentPath
	n.mu.RUnlock()
	if currentPath == "" || currentPath != path {
		driver.Log.Infof("syncFile: aborting sync for %s (node detached or path changed)\n", path)
		n.mu.Lock()
		n.syncQueued = false
		n.mu.Unlock()
		return nil
	}

	defer func() {
		stats.TotalDuration = time.Since(startedAt)
		fs.notifySyncFinish(stats, err)
		n.mu.Lock()
		n.syncQueued = false
		// 核心加固：如果同步完成后文件依然是脏的（说明上传期间有新写入），立即补发同步
		isStillDirty := n.isDirty
		n.mu.Unlock()

		if isStillDirty && err == nil {
			driver.Log.Infof("syncFile: %s still dirty after sync, re-enqueuing...\n", path)
			fs.enqueueSync(n)
		}
	}()

	n.mu.Lock()
	if !n.isDirty {
		n.mu.Unlock()
		return nil
	}

	// 核心改进：在上传前最后一次确认物理文件大小
	// 解决 macOS FUSE 先 Release 后 Write 导致的 0 字节卡死问题
	if n.localPath != "" && fs.staging != nil {
		if actualSize, err := fs.staging.FileSize(n.localPath); err == nil {
			if actualSize > 0 && n.size != actualSize {
				driver.Log.Infof("syncFile: refreshing size for %s from staging (%d -> %d)\n", path, n.size, actualSize)
				n.size = actualSize
			}
		}
	}

	snapshotSize := n.size
	snapshotName := n.name
	snapshotMtime := n.mtime
	parentFid := n.parentFid
	fid := n.fid
	baseMtime := n.baseServerMtime
	localPath := n.localPath
	lastUpload := n.lastUploadTime
	oldUploadedFid := n.uploadedFid // 记录上次上传的 FID，用于 FID 直接替换
	uploadID := n.uploadID          // 断点续传的 upload_id
	n.mu.Unlock()

	// [DEBUG] 上传前状态快照，用于排查 (1) 重名问题
	var stagingFileSize int64
	if localPath != "" && fs.staging != nil {
		if sz, err := fs.staging.FileSize(localPath); err == nil {
			stagingFileSize = sz
		}
	}
	driver.Log.Debugf("syncFile path=%s snapshotSize=%d stagingFileSize=%d fid=%s parentFid=%s localPath=%s\n", path, snapshotSize, stagingFileSize, fid, parentFid, localPath)
	if stagingFileSize == 0 && snapshotSize > 0 {
		driver.Log.Warnf("[BUG] syncFile: staging is empty but size=%d for %s (200ms delay insufficient?)\n", snapshotSize, path)
	}

	// Guard: skip re-sync if this file was just uploaded (< 10s ago) and has a real server FID.
	if !strings.HasPrefix(fid, "local_") && !lastUpload.IsZero() && time.Since(lastUpload) < 10*time.Second {
		return nil
	}
	stats.SnapshotSize = snapshotSize

	// 1b. Check for same-name conflict on server
	if !strings.HasPrefix(fid, "local_") {
		files, listErr := fs.driver.ListFiles(parentFid)
		if listErr == nil {
			for _, f := range files {
				if f.Fid == fid {
					continue
				}
				decName, _ := fs.cipher.DecryptSegment(f.FileName)
				if decName == snapshotName {
					driver.Log.Infof("Sync: CONFLICT (same name, different FID) for %s (current=%s, remote=%s). Resolving...\n", path, fid, f.Fid)
					fs.resolveConflict(path, n, f)
					return nil
				}
			}
		}
	}

	// 1c. Pre-upload Conflict Check
	if !strings.HasPrefix(fid, "local_") {
		rf, err := fs.fileExistsOnServerDetailed(fid, parentFid)
		if err != nil {
			return fmt.Errorf("pre-upload check failed: %v", err)
		}
		if rf == nil {
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
					driver.Log.Infof("Sync: CONFLICT (FID gone but name exists) for %s. Resolving...\n", path)
					fs.resolveConflict(path, n, *foundRf)
					return nil
				}
			}
			driver.Log.Infof("Sync: FID %s gone from server, restarting as new file\n", fid)
			fid = "local_" + n.name
		} else {
			if rf.ModTime().UnixMilli() > baseMtime {
				driver.Log.Infof("Sync: CONFLICT (server mtime %d > base %d) for %s. Resolving...\n", rf.ModTime().UnixMilli(), baseMtime, path)
				fs.resolveConflict(path, n, *rf)
				return nil
			}
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

	fs.notifySyncStart(path, n)
	result, err := fs.uploader.Sync(uploadpkg.SyncRequest{
		Path:      path,
		Name:      snapshotName,
		ParentFid: parentFid,
		LocalPath: localPath,
		PlainSize: snapshotSize,
		OldFid:    oldUploadedFid, // FID 直接替换，绕过 ListFiles 索引延迟
		Nonce:     n.fileNonce,    // 断点续传：复用崩溃前的 nonce（同 nonce = 同加密数据）
		UploadID:  uploadID,       // 断点续传：复用崩溃前的 upload session
	})

	// 保存 upload_id 用于断点续传（UploadPre 成功后已有值，无论 Sync 后续是否成功）
	if result.UploadID != "" && fs.cache != nil {
		n.mu.Lock()
		n.uploadID = result.UploadID
		n.mu.Unlock()
		_ = fs.cache.UpdatePendingNodeUpload(path, result.UploadID)
	}

	// [DEBUG] Sync 结果，用于排查 (1) 重名问题
	if err != nil {
		driver.Log.Debugf("syncFile Sync FAILED for %s: %v\n", path, err)
	} else {
		driver.Log.Debugf("syncFile Sync OK for %s: resultFid=%s encSize=%d\n", path, result.Fid, result.EncryptedSize)
	}

	if err != nil {
		if errors.Is(err, errDirGone) {
			driver.Log.Infof("Sync: parent directory %s gone, will retry recreation\n", parentFid)
			if v, ok := fs.nodes.Load(filepath.Dir(path)); ok {
				pn := v.(*node)
				pn.mu.RLock()
				newParentFid := pn.fid
				pn.mu.RUnlock()
				if newParentFid != parentFid {
					n.mu.Lock()
					n.parentFid = newParentFid
					n.mu.Unlock()
				}
			}
			return errDirGone
		}
		return err
	}

	n.mu.Lock()
	oldFid := n.fid
	n.fid = result.Fid
	n.expectedFid = result.Fid
	// 不在此处设置 source="remote"。原因：
	// 上传成功后夸克 API 可能还未索引到文件，如果立即标记为 "remote"，
	// MergeRemoteChanges 在 Readdir 时发现文件不在远程列表中，会误判为"远程已删除"并删除本地节点。
	// source 的 "local" → "remote" 转换由 MergeRemoteChanges 在远程列表中确认文件存在后自动完成
	// （path_state.go 中 exists && rf.Fid == expectedFid 分支）。
	n.fileNonce = result.Nonce
	n.hasNonce = true
	n.encSize = result.EncryptedSize
	n.uploadedFid = result.Fid // 记录上传后的 FID，用于下次 re-upload 时 FID 直接替换
	// 上传成功即清除 dirty（无条件），不比较 mtime。
	// 设计：Release 是唯一的 sync 触发点，上传时所有 Write 已完成。
	// 如果上传期间有新 Write（文件被重新打开），Write 会重新设置 isDirty=true，
	// 下一次 Release 会再次触发 sync。
	n.isDirty = false
	n.baseServerMtime = snapshotMtime.UnixMilli()
	n.baseServerSize = n.size
	n.lastMetadataCheck = time.Now()
	n.lastUploadTime = time.Now()
	localPath = n.localPath
	// 锁内清理 DB + staging，缩小崩溃窗口
	// isDirty=false 与 DB 清理在同一锁内完成——要么同时生效，要么都不生效
	if fs.cache != nil {
		_ = fs.cache.RemovePendingNode(path)
		if n.currentPath != "" && n.currentPath != path {
			_ = fs.cache.RemovePendingNode(n.currentPath)
		}
	}
	n.localPath = ""
	if fs.staging != nil && localPath != "" {
		_ = fs.staging.Remove(localPath)
	}
	newFid := n.fid
	n.mu.Unlock()

	// Update global FID index
	if oldFid != "" && oldFid != newFid {
		fs.fidNodes.Delete(oldFid)
	}
	if newFid != "" && !strings.HasPrefix(newFid, "local_") {
		fs.fidNodes.Store(newFid, n)
	}

	return nil
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

func (fs *QryptFS) cleanupLocalUploadState(path string, n *node, recursive bool) {
	if n == nil {
		return
	}
	n.mu.Lock()
	fid := n.fid
	localPath := n.localPath
	n.mu.Unlock()

	if fs.cache != nil {
		if recursive {
			_ = fs.cache.RemovePendingNodesByPrefix(path)
		} else {
			_ = fs.cache.RemovePendingNode(path)
			if fid != "" {
				_ = fs.cache.RemovePendingNodesByFid(fid)
			}
		}
	}
	if fs.staging != nil && localPath != "" {
		_ = fs.staging.Remove(localPath)
	}
}

func (fs *QryptFS) enqueueSync(n *node) {
	fs.enqueueSyncDelay(n, 0)
}

// enqueueSyncDelay 带延迟的入队。delay>0 时通过 goroutine 延迟投递，
// 给 macOS FUSE Release-before-Write 场景留出 Write 写入 staging 的时间。
// defer re-enqueue 调用时 delay=0（立即投递）。
func (fs *QryptFS) enqueueSyncDelay(n *node, delay time.Duration) {
	n.mu.RLock()
	// 核心修复：即使 isDirty 为 false，如果 FID 还是 local_（代表新创建且从未同步），也允许排队
	isNewLocal := strings.HasPrefix(n.fid, "local_")
	if (!n.isDirty && !isNewLocal) || n.syncQueued {
		n.mu.RUnlock()
		return
	}
	n.mu.RUnlock()

	n.mu.Lock()
	if n.syncQueued {
		n.mu.Unlock()
		return
	}
	n.syncQueued = true
	n.mu.Unlock()

	if delay > 0 {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					driver.Log.Errorf("PANIC in enqueueSyncDelay: %v\n%s\n", r, debug.Stack())
				}
			}()
			time.Sleep(delay)
			fs.uploadChan <- syncTask{node: n}
		}()
	} else {
		fs.uploadChan <- syncTask{node: n}
	}
}
