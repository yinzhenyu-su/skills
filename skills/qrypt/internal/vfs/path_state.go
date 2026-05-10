package vfs

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/driver"
)

func isFinderTrashDir(path string) bool {
	clean := filepath.Clean(path)
	if clean == "/.Trash" || clean == "/.Trashes" {
		return true
	}

	parts := strings.Split(strings.Trim(clean, "/"), "/")
	return len(parts) == 2 && (parts[0] == ".Trash" || parts[0] == ".Trashes")
}

func isFinderTrashPath(path string) bool {
	for _, part := range strings.Split(strings.Trim(filepath.Clean(path), "/"), "/") {
		if part == ".Trash" || part == ".Trashes" {
			return true
		}
	}
	return false
}

func fillVirtualTrashStat(stat *fuse.Stat_t) {
	uid, gid, _ := fuse.Getcontext()
	now := fuse.NewTimespec(time.Now())
	stat.Uid = uid
	stat.Gid = gid
	stat.Mode = fuse.S_IFDIR | 0755
	stat.Nlink = 2
	stat.Mtim = now
	stat.Atim = now
	stat.Ctim = now
}

func (fs *QryptFS) currentPathForNode(n *node) string {
	n.mu.RLock()
	path := n.currentPath
	parentFid := n.parentFid
	name := n.name
	n.mu.RUnlock()

	if path != "" {
		return path
	}

	// 如果路径丢失，通过 parentFid 递归向上构建，避免 O(N) 扫描
	constructedPath := ""
	if parentFid == "" || parentFid == fs.rootFid {
		if name == "" {
			constructedPath = "/"
		} else {
			constructedPath = "/" + name
		}
	} else {
		// 找到父节点
		var parentNode *node
		if v, ok := fs.fidNodes.Load(parentFid); ok {
			parentNode = v.(*node)
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

	// 验证构建的路径是否确实指向我们 (如果已被删除，不应返回假路径)
	if constructedPath != "" {
		if v, ok := fs.nodes.Load(constructedPath); ok && v.(*node) == n {
			n.mu.Lock()
			n.currentPath = constructedPath
			n.mu.Unlock()
			return constructedPath
		}
	}

	return ""
}

func (fs *QryptFS) storeNode(path string, n *node) {
	path = filepath.Clean(path)
	n.mu.Lock()
	n.currentPath = path
	if n.isFolder && n.children == nil {
		n.children = make(map[string]*node)
	}
	n.mu.Unlock()

	fs.nodes.Store(path, n)

	// 维护父子引用，避免 O(N) 扫描
	if path != "/" {
		parentPath := filepath.Dir(path)
		if v, ok := fs.nodes.Load(parentPath); ok {
			p := v.(*node)
			p.mu.Lock()
			if p.children == nil {
				p.children = make(map[string]*node)
			}

			// --- 核心修复：防止自引用循环 ---
			if n.fid != "" && n.fid == p.fid {
				driver.Log.Infof("CRITICAL: detected self-reference attempt for path %s (FID %s). Blocking.\n", path, n.fid)
				p.mu.Unlock()
				return
			}
			// -----------------------------

			p.children[filepath.Base(path)] = n
			p.mtime = time.Now() // 更新父目录修改时间
			p.mu.Unlock()
		}
	}

	// 维护 fid 索引 (仅针对远程节点)
	n.mu.RLock()
	fid := n.fid
	n.mu.RUnlock()
	if fid != "" && !strings.HasPrefix(fid, "local_") {
		fs.fidNodes.Store(fid, n)
	}
}

func (fs *QryptFS) replaceNodePath(oldPath, newPath string, n *node) {
	if oldPath != newPath {
		fs.nodes.Delete(oldPath)

		// 更新父子关系
		oldParent := filepath.Dir(oldPath)
		newParent := filepath.Dir(newPath)
		if oldParent != newParent {
			// 移除旧父节点引用
			if v, ok := fs.nodes.Load(oldParent); ok {
				p := v.(*node)
				p.mu.Lock()
				if p.children != nil {
					delete(p.children, filepath.Base(oldPath))
				}
				p.mu.Unlock()
			}
			// 添加到新父节点 (storeNode 会做，但我们这里显式处理逻辑更清晰)
		} else if oldPath != "" {
			// 仅仅是同目录下重命名
			if v, ok := fs.nodes.Load(oldParent); ok {
				p := v.(*node)
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
		n.children = make(map[string]*node)
	}
	n.mu.Unlock()

	fs.nodes.Store(newPath, n)

	// 如果父目录变了，确保链入新父目录
	if oldPath != newPath {
		parentPath := filepath.Dir(newPath)
		if v, ok := fs.nodes.Load(parentPath); ok {
			p := v.(*node)
			p.mu.Lock()
			if p.children == nil {
				p.children = make(map[string]*node)
			}
			p.children[filepath.Base(newPath)] = n
			p.mu.Unlock()
		}
	}
}

func (fs *QryptFS) deleteNodePath(path string, n *node) {
	path = filepath.Clean(path)
	fs.nodes.Delete(path)

	// 从父节点移除引用
	if path != "/" {
		parentPath := filepath.Dir(path)
		if v, ok := fs.nodes.Load(parentPath); ok {
			p := v.(*node)
			// --- 核心修复：使用 TryLock 更新修改时间，防止删除过程中的 ABBA 死锁 ---
			if p.mu.TryLock() {
				if p.children != nil {
					delete(p.children, filepath.Base(path))
				}
				p.mtime = time.Now()
				p.mu.Unlock()
			} else {
				// 如果拿不到锁，至少在不持有锁的情况下从 map 移除引用
				// 注意：sync.Map 本身是线程安全的，这里主要保护的是 p.children map
				// 我们需要一个更安全的方式来处理 children map 的并发删除
				fs.safeRemoveChild(p, filepath.Base(path))
			}
		}
	}

	// 内存中状态清理逻辑：不再锁定子节点 n，因为正在删除中，最小化锁冲突
	var fid string
	var localPath string
	var isFolder bool
	if n != nil {
		n.mu.RLock()
		fid = n.fid
		localPath = n.localPath
		isFolder = n.isFolder
		n.mu.RUnlock()

		if n.currentPath == path {
			n.currentPath = ""
			n.cancel() // 原子标记取消，拦截待上传任务
			fs.retryState.Delete(n) // 清理重试计数，避免重试 goroutine 继续尝试已删除的节点
		}

		// 如果是普通文件且有本地 staging，立即物理清理（不依赖 API）
		if localPath != "" && fs.staging != nil {
			_ = fs.staging.Remove(localPath)
		}
		
		// 如果是目录，仅执行内存树的递归清理，绝对不产生额外的异步 API 任务
		if isFolder {
			go fs.deleteSubtreePaths(path, n)
		}
	}

	if fid != "" {
		// 内存索引先行移除
		if !strings.HasPrefix(fid, "local_") {
			fs.fidNodes.Delete(fid)
		}
	}
}

func (fs *QryptFS) deleteSubtreePaths(parentPath string, n *node) {
	if n == nil || !n.isFolder {
		return
	}

	prefix := parentPath
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

	var fidsToPurge []string
	var pathsToPurge []string

	for _, c := range children {
		childPath := prefix + c.name

		// 核心加固：对象级精准清理
		// 只有当内存中该路径对应的依然是我们要删除的那个旧节点时，才执行清理
		if v, ok := fs.nodes.Load(childPath); ok && v.(*node) == c.child {
			fs.nodes.Delete(childPath)
		}

		if c.child != nil {
			c.child.mu.RLock()
			fid := c.child.fid
			isFolder := c.child.isFolder
			localPath := c.child.localPath
			c.child.mu.RUnlock()

			if fid != "" && !strings.HasPrefix(fid, "local_") {
				// 只有当 fidNodes 指向的也是同一个对象时才删除索引
				if v, ok := fs.fidNodes.Load(fid); ok && v.(*node) == c.child {
					fs.fidNodes.Delete(fid)
				}
				fidsToPurge = append(fidsToPurge, fid)
				// 物理清理磁盘分块
				if fs.cache != nil {
					_ = fs.cache.RemoveChunksByFid(fid)
				}
			}

			// 如果有正在排队的上传，清理 staging
			if localPath != "" && fs.staging != nil {
				_ = fs.staging.Remove(localPath)
			}

			if isFolder {
				fs.deleteSubtreePaths(childPath, c.child)
			}
		}
	}

	// 批量清理数据库状态
	if fs.cache != nil && (len(fidsToPurge) > 0 || len(pathsToPurge) > 0) {
		_ = fs.cache.BatchDeleteNodeState(fidsToPurge, pathsToPurge)
		if db, ok := fs.cache.GetDB().(*cache.CacheDB); ok {
			_ = db.BatchMarkOpsDone(fidsToPurge, pathsToPurge)
		}
	}
}

// safeRemoveChild 尝试安全地从父节点移除子节点引用。
// 先以 10μs 间隔自旋 100 次（共 1ms），仍失败则走阻塞锁。
func (fs *QryptFS) safeRemoveChild(p *node, baseName string) {
	go func() {
		for i := 0; i < 100; i++ {
			if p.mu.TryLock() {
				if p.children != nil {
					delete(p.children, baseName)
				}
				p.mtime = time.Now()
				p.mu.Unlock()
				return
			}
			time.Sleep(10 * time.Microsecond)
		}
		// 最终手段：强制锁定（此时风险已降级）
		p.mu.Lock()
		if p.children != nil {
			delete(p.children, baseName)
		}
		p.mu.Unlock()
	}()
}

func (fs *QryptFS) persistPendingPath(oldPath, newPath string, n *node) {
	if fs.cache == nil || n == nil {
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

	_ = fs.cache.SavePendingNode(newPath, fid, parentFid, name, localPath, size, false, nonce, baseMtime, baseSize)
	if oldPath != "" && oldPath != newPath {
		_ = fs.cache.RemovePendingNode(oldPath)
	}
}

func (fs *QryptFS) renameSubtreePaths(oldPath, newPath string) {
	if v, ok := fs.nodes.Load(oldPath); ok {
		n := v.(*node)
		fs.recursiveRename(oldPath, newPath, n)
	}
}

func (fs *QryptFS) recursiveRename(oldPath, newPath string, n *node) {
	// 1. 更新当前节点路径
	fs.replaceNodePath(oldPath, newPath, n)
	fs.persistPendingPath(oldPath, newPath, n)

	if !n.isFolder {
		return
	}

	// 2. 递归更新子节点
	prefixOld := oldPath
	if !strings.HasSuffix(prefixOld, "/") {
		prefixOld += "/"
	}
	prefixNew := newPath
	if !strings.HasSuffix(prefixNew, "/") {
		prefixNew += "/"
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
		fs.recursiveRename(prefixOld+c.name, prefixNew+c.name, c.child)
	}
}

func (fs *QryptFS) lookup(path string) (*node, int) {
	return fs.lookupExtended(path, true)
}

func (fs *QryptFS) lookupExtended(path string, refresh bool) (*node, int) {
	path = filepath.Clean(path)
	// --- 核心修复：拦截正在删除的路径 ---
	if fs.isUnderDeletingDir(path) {
		return nil, -fuse.ENOENT
	}
	// ---------------------------------

	if v, ok := fs.nodes.Load(path); ok {
		n := v.(*node)

		n.mu.RLock()
		isFolder := n.isFolder
		lastCheck := n.lastMetadataCheck
		isDirty := n.isDirty
		fid := n.fid
		parentFid := n.parentFid
		n.mu.RUnlock()

		if !strings.HasPrefix(fid, "local_") {
			if refresh && !isFolder && !isDirty && time.Since(lastCheck) > MetadataTTL {
				// 如果已经是墓碑，直接返回 ENOENT
				if _, inDeletion := fs.activeDeletions.Load(fid); inDeletion {
					fs.deleteNodePath(path, n)
					return nil, -fuse.ENOENT
				}

				// Refresh single file metadata + verify existence in one ListFiles call
				files, err := fs.fetchFiles(parentFid)
				found := false
				if err == nil {
					for _, f := range files {
						if f.Fid == fid {
							// 还是检查一次是否变成了墓碑
							if _, inDeletion := fs.activeDeletions.Load(fid); inDeletion {
								break
							}

							decSize, _ := fs.cipher.DecryptedSize(f.Int64Size())
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

					driver.Log.Infof("lookup: file %s (fid=%s) deleted from server, removing from cache\n", path, fid)
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

		// 如果缓存中有，直接用
		if v, ok := fs.nodes.Load(currentPath); ok {
			currentFid = v.(*node).fid
			continue
		}

		// --- 再次拦截正在删除的中间路径 ---
		if fs.isUnderDeletingDir(currentPath) {
			return nil, -fuse.ENOENT
		}
		// ---------------------------------

		// 否则，列出父目录内容来寻找
		// --- 核心修复：检查当前父目录是否是墓碑 ---
		if _, inDeletion := fs.activeDeletions.Load(currentFid); inDeletion {
			return nil, -fuse.ENOENT
		}
		// ---------------------------------------

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

			// --- 核心修复：防止幽灵复活 ---
			if _, inDeletion := fs.activeDeletions.Load(f.Fid); inDeletion {
				found = false // 故意不设置 found，返回 ENOENT
				break
			}
			// --------------------------

			// 保护逻辑：如果本地已有该路径的 Dirty 节点，不覆盖它
			if v, ok := fs.nodes.Load(currentPath); ok {
				existing := v.(*node)
				existing.mu.RLock()
				dirty := existing.isDirty
				existing.mu.RUnlock()
				if dirty {
					currentFid = existing.fid
					found = true
					break
				}
			}

			decSize, _ := fs.cipher.DecryptedSize(f.Int64Size())
			modTime := f.ModTime()
			driver.Log.Infof("[FUSE] lookup: creating node for '%s' (FID='%s') with parentFid='%s'\n", decName, f.Fid, currentFid)
			lastCheck := time.Time{}
			if !f.IsDir() {
				lastCheck = time.Now()
			}
			newNode := &node{
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
		return v.(*node), 0
	}
	return nil, -fuse.ENOENT
}

type opsPayload struct {
	Fid           string   `json:"fid,omitempty"`
	Fids          []string `json:"fids,omitempty"`
	ParentFid     string   `json:"parent_fid,omitempty"`
	CurrentDirFid string   `json:"current_dir_fid,omitempty"`
	Name          string   `json:"name,omitempty"`
}

func (fs *QryptFS) recoverPendingOps() {
	if fs.cache == nil {
		return
	}
	db, ok := fs.cache.GetDB().(*cache.CacheDB)
	if !ok || db == nil {
		return
	}
	logs, err := db.GetPendingOpsLogs()
	if err != nil {
		driver.Log.Errorf("recoverPendingOps: failed to get logs: %v\n", err)
		return
	}

	for _, l := range logs {
		driver.Log.Infof("recoverPendingOps: retrying %s (id=%d) %s -> %s\n", l.OpType, l.ID, l.SourcePath, l.TargetPath)
		var p opsPayload
		_ = json.Unmarshal([]byte(l.Payload), &p)

		var err error
		switch l.OpType {
		case "MKDIR":
			_, err = fs.driver.CreateDir(p.ParentFid, p.Name)
		case "UNLINK", "RMDIR", "DELETE":
			deleteFids := p.Fids
			if len(deleteFids) == 0 && p.Fid != "" {
				deleteFids = []string{p.Fid}
			}
			if len(deleteFids) > 0 {
				err = fs.driver.Delete(deleteFids)
			}
		case "RENAME":
			if p.ParentFid != "" {
				_ = fs.driver.Move(p.Fids, p.ParentFid, p.CurrentDirFid)
			}
			err = fs.driver.Rename(p.Fids[0], p.Name)
		}

		if err == nil {
			driver.Log.Infof("recoverPendingOps: retry %d (%s) SUCCEEDED\n", l.ID, l.OpType)
			_ = db.UpdateOpsLogStatus(l.ID, "DONE")
		} else {
			msg := err.Error()
			if strings.Contains(msg, driver.QuarkErrAlreadyDeleted) || strings.Contains(msg, "404") || strings.Contains(msg, "not found") {
				driver.Log.Infof("recoverPendingOps: retry %d (%s) SUCCEEDED (already deleted)\n", l.ID, l.OpType)
				_ = db.UpdateOpsLogStatus(l.ID, "DONE")
			} else {
				driver.Log.Errorf("recoverPendingOps: retry %d failed: %v\n", l.ID, err)
			}
		}
	}
}

func (fs *QryptFS) MergeRemoteChanges(parentPath string, parentFid string, remoteFiles []driver.File) {
	if parentFid == "" {
		return
	}

	// 1. 使用 merging 锁进行合并操作去重，防止同一目录被并发执行多次 Merge（高并发刷新/预取场景）
	waitChan, loading := fs.merging.LoadOrStore(parentFid, make(chan struct{}))
	if loading {
		<-waitChan.(chan struct{})
		return
	}
	defer func() {
		close(waitChan.(chan struct{}))
		fs.merging.Delete(parentFid)
	}()

	// Skip if this directory (or an ancestor) is being deleted — prevents re-adding children
	// 同时如果在 Rmdir 保护期内，也直接跳过更新，防止删了又加
	if fs.isUnderDeletingDir(parentPath) {
		driver.Log.Infof("MergeRemoteChanges: skipping %s (directory or ancestor being deleted)\n", parentPath)
		return
	}

	// --- 核心修复：检查父目录本身是否已在删除队列中 ---
	if _, inDeletion := fs.activeDeletions.Load(parentFid); inDeletion {
		driver.Log.Infof("MergeRemoteChanges: skipping %s (parent FID %s is tombstoned)\n", parentPath, parentFid)
		return
	}
	// ---------------------------------------------

	seenFids := make(map[string]bool)
	remoteMap := make(map[string]driver.File)
	remoteFids := make(map[string]bool)

	for _, f := range remoteFiles {
		// --- 核心修复：过滤自我引用，防止幽灵双胞胎 ---
		if f.Fid == parentFid {
			continue
		}
		// ----------------------------------------

		seenFids[f.Fid] = true
		remoteFids[f.Fid] = true

		decName := ""
		// 1. 优先使用缓存中已知的解密名称
		if v, ok := fs.fidNodes.Load(f.Fid); ok {
			pn := v.(*node)
			pn.mu.RLock()
			decName = pn.name
			pn.mu.RUnlock()
		}
		// 2. 如果内存没有，尝试从持久化数据库缓存中读取
		if decName == "" && fs.cache != nil {
			if cached, ok, _ := fs.cache.GetCachedName(f.Fid, f.FileName); ok {
				decName = cached
			}
		}
		// 3. 如果还是未知，则进行昂贵的解密并存入缓存
		if decName == "" {
			decName, _ = fs.cipher.DecryptSegment(f.FileName)
			if decName != "" && fs.cache != nil {
				_ = fs.cache.SaveCachedName(f.Fid, f.FileName, decName)
			}
		}

		// --- 核心修复：禁止非法名称 ---
		if decName == "" || decName == "." || decName == ".." {
			continue
		}
		// ----------------------------

		// 跳过同名文件（夸克网盘允许 xxx 和 xxx(1) 共存，解密后可能重名）
		if _, exists := remoteMap[decName]; exists {
			driver.Log.Infof("MergeRemoteChanges: skipping duplicate remote file '%s' (fid=%s) in %s\n", decName, f.Fid, parentPath)
			if strings.Contains(f.FileName, "(") && strings.HasSuffix(f.FileName, ")") {
				driver.Log.Warnf("MergeRemoteChanges: conflict copy detected — '%s' (fid=%s) is a duplicate of same-name file (decrypted=%s)\n", f.FileName, f.Fid, decName)
			}
			continue
		}
		remoteMap[decName] = f
	}

	// 1. 处理本地已有的节点：更新或冲突检测
	prefix := parentPath
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	type nodeEntry struct {
		path string
		node *node
	}
	var localEntries []nodeEntry

	// 通过 parentNode.children 直接获取子节点，避免全局 O(N) 扫描
	if v, ok := fs.nodes.Load(parentPath); ok {
		p := v.(*node)
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

		// 跳过正在同步的文件，记录数量但不逐个打印日志（高频场景下会刷屏）
		if syncQueued {
			syncInProgressCount++
			continue
		}

		rf, exists := remoteMap[name]

		// 改进：引入 ExpectedFID 追踪（彻底消除 30s 依赖）
		if exists && rf.Fid == expectedFid {
			n.mu.Lock()
			if n.fid != expectedFid {
				driver.Log.Infof("MergeRemoteChanges: %s matched expectedFid %s, updating node FID and source\n", entry.path, expectedFid)
				n.fid = expectedFid
			}
			n.source = "remote"
			n.mu.Unlock()
			// 既然已经匹配到预期的 FID，继续后续元数据更新流程
		}

		// 刚上传完成的文件，跳过以防 API 索引延迟导致误判为"远程删除"
		// 这里优先信任 local 状态，如果 source 没设置，回退到时间判断
		if !isDirty && (n.source == "local" || n.source == "merged" || (!lastUpload.IsZero() && time.Since(lastUpload) < 30*time.Second)) {
			// 如果还没在远程列表中确认过，即便列表里没有，也继续保留
			if !exists {
				driver.Log.Infof("MergeRemoteChanges: %s (source=%s, lastUpload=%v) not in remote list yet, keeping local node\n", entry.path, n.source, lastUpload)
				continue
			}
			// 如果在列表中看到了，且 FID 一致，说明已索引，转为 remote 状态
			if rf.Fid == fid {
				n.mu.Lock()
				if n.source != "remote" {
					n.source = "remote"
					driver.Log.Infof("MergeRemoteChanges: %s confirmed on server, transitioned to remote source\n", entry.path)
				}
				n.mu.Unlock()
			}
		}

		if !exists {
			// 远端不存在该文件
			if isDirty {
				driver.Log.Infof("MergeRemoteChanges: %s is dirty but not on remote, keeping local\n", entry.path)
				continue
			}
			if n.source == "local" || n.source == "merged" {
				// 本地新建或冲突合并中的文件，且上面没被 rf.Fid == fid 匹配到（说明 FID 变了或确实没索引）
				driver.Log.Infof("MergeRemoteChanges: skipping delete for local-owned file %s\n", entry.path)
				continue
			}
			// 未上传过的本地文件（fid 以 local_ 开头），跳过删除
			// source 字段在部分创建路径（ensureParentDirExists, resolveConflict 等）未设置，
			// 仅靠 source=="local" 不够可靠，fid 前缀是更直接的判断依据
			if strings.HasPrefix(fid, "local_") {
				driver.Log.Infof("MergeRemoteChanges: skipping delete for local_ fid file %s (fid=%s)\n", entry.path, fid)
				continue
			}

			// 只有 source == "remote" 的文件，才完全相信远程列表的“不存在”即为“已删除”
			fs.deleteNodePath(entry.path, n)
			remoteDeletedCount++
			continue
		}

		// 远端存在
		if !strings.HasPrefix(fid, "local_") {
			if rf.Fid != fid {
				// FID 变了（可能是删了重建），按更新处理
				driver.Log.Infof("MergeRemoteChanges: FID changed for %s (%s -> %s)\n", entry.path, fid, rf.Fid)
			}

			remoteMtime := rf.ModTime().UnixMilli()
			n.mu.RLock()
			source := n.source
			n.mu.RUnlock()

			if source == "remote" && baseMtime > 0 && remoteMtime > baseMtime+2000 {
				if !isDirty {
					// 纯远端更新
					decSize, _ := fs.cipher.DecryptedSize(rf.Int64Size())
					n.mu.Lock()
					n.fid = rf.Fid
					n.size = decSize
					n.encSize = rf.Int64Size()
					n.mtime = rf.ModTime()
					n.baseServerMtime = remoteMtime
					n.baseServerSize = decSize
					n.lastMetadataCheck = time.Now()
					n.source = "remote"
					n.mu.Unlock()
					// 失效缓存
					if fs.cache != nil {
						_ = fs.cache.RemoveChunksByFid(fid)
					}
					driver.Log.Infof("MergeRemoteChanges: updated %s from server\n", entry.path)
				} else {
					// 冲突：双向改
					driver.Log.Infof("MergeRemoteChanges: CONFLICT (both modified) for %s. Triggering side-by-side rename.\n", entry.path)
					fs.resolveConflict(entry.path, n, rf)
				}
			} else if isDirty {
				// source 不是 "remote"（如 "local"/"merged"），但本地有未上传修改且远端同名 → 冲突
				driver.Log.Infof("MergeRemoteChanges: CONFLICT (local dirty, remote exists) for %s. Triggering side-by-side rename.\n", entry.path)
				fs.resolveConflict(entry.path, n, rf)
			}
		} else {
			// 本地是 local_，但远端出现了同名文件（可能是别人上传了同名文件）
			// 但如果刚上传过（30s 内），远程文件很可能是我们自己的上传，
			// 因为 syncFile 在 API 未索引时将 fid 转回了 local_。
			if !lastUpload.IsZero() && time.Since(lastUpload) < 30*time.Second {
				driver.Log.Infof("MergeRemoteChanges: skipping conflict for %s (just uploaded %dms ago, remote file likely ours)\n", entry.path, time.Since(lastUpload).Milliseconds())
				continue
			}
			driver.Log.Infof("MergeRemoteChanges: CONFLICT (local new, remote exists) for %s. Triggering side-by-side rename.\n", entry.path)
			fs.resolveConflict(entry.path, n, rf)
		}
	}

	if syncInProgressCount > 0 {
		driver.Log.Infof("MergeRemoteChanges: skipped %d files in %s (sync in progress)\n", syncInProgressCount, parentPath)
	}
	if remoteDeletedCount > 0 {
		driver.Log.Infof("MergeRemoteChanges: removed %d local nodes in %s (remote deleted)\n", remoteDeletedCount, parentPath)
	}

	// 2. 处理远端有但本地没有的新文件
	addedCount := 0
	for name, rf := range remoteMap {
		if seenLocalNames[name] {
			continue
		}

		// 证据驱动的“墓碑”机制：
		// 如果该 FID 在删除队列中，我们绝对不把它作为“新文件”加回来。
		if _, inDeletion := fs.activeDeletions.Load(rf.Fid); inDeletion {
			driver.Log.Infof("MergeRemoteChanges: blocking resurrected zombie file %s (fid=%s)\n", name, rf.Fid)
			continue
		}

		childPath := prefix + name
		decSize, _ := fs.cipher.DecryptedSize(rf.Int64Size())
		modTime := rf.ModTime()
		fs.storeNode(childPath, &node{
			fid:               rf.Fid,
			parentFid:         parentFid,
			name:              name,
			size:              decSize,
			encSize:           rf.Int64Size(),
			currentPath:       childPath,
			isFolder:          rf.IsDir(),
			mtime:             modTime,
			baseServerMtime:   modTime.UnixMilli(),
			baseServerSize:    decSize,
			lastMetadataCheck: time.Now(),
			source:            "remote",
		})
		addedCount++
	}
	if addedCount > 0 {
		driver.Log.Infof("MergeRemoteChanges: added %d new remote files to %s\n", addedCount, parentPath)
	}

	// 3. 证据驱动的清理：
	// 只检查当前目录下的子墓碑
	if val, ok := fs.deletionsByParent.Load(parentFid); ok {
		childMap := val.(*sync.Map)
		childMap.Range(func(key, value interface{}) bool {
			fid := key.(string)
			if stateVal, exists := fs.activeDeletions.Load(fid); exists {
				state := stateVal.(*deletionState)
				if state.apiDone {
					if !remoteFids[fid] {
						// 证据：API 已调成功 + 远端列表已不包含该 FID = 索引已同步
						driver.Log.Infof("MergeRemoteChanges: evidence confirmed - FID %s is gone from server list, clearing tombstone and path protection\n", fid)
						
						// 同时清除路径保护
						if state.path != "" {
							fs.deletingPaths.Delete(state.path)
						}

						fs.activeDeletions.Delete(fid)
						childMap.Delete(fid)

						// 递归清理逻辑：如果刚消失的是一个目录，将其下的所有后代墓碑也清理掉
						// 因为父目录都没了，我们永远不会再刷新它来获取后代消失的证据。
						fs.purgeTombstonesRecursively(fid)
					}
				}
			} else {
				// 状态不对称，清理索引
				childMap.Delete(fid)
			}
			return true
		})
	}
}

// purgeTombstonesRecursively 递归清理某个 FID 下的所有子孙墓碑
func (fs *QryptFS) purgeTombstonesRecursively(parentFid string) {
	if parentFid == "" {
		return
	}

	if val, ok := fs.deletionsByParent.Load(parentFid); ok {
		childMap := val.(*sync.Map)
		childMap.Range(func(key, value interface{}) bool {
			fid := key.(string)
			// 防环检查
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
			fs.purgeTombstonesRecursively(fid) // 继续向下递归
			return true
		})
		fs.deletionsByParent.Delete(parentFid)
	}
}

func (fs *QryptFS) resolveConflict(path string, n *node, rf driver.File) {
	// 1. 重命名本地脏节点
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	conflictPath := fmt.Sprintf("%s [Local Conflict %s]%s", base, time.Now().Format("20060102_150405"), ext)

	driver.Log.Infof("resolveConflict: Renaming local %s -> %s\n", path, conflictPath)

	n.mu.Lock()
	newName := filepath.Base(conflictPath)
	// 强制变为 local_ fid 以重新上传
	if !strings.HasPrefix(n.fid, "local_") {
		n.fid = "local_" + newName + "_" + fmt.Sprint(time.Now().UnixNano())
	}
	n.name = newName
	n.mu.Unlock()

	fs.replaceNodePath(path, conflictPath, n)
	fs.persistPendingPath(path, conflictPath, n)

	// 2. 为原路径拉取远端节点
	decSize, _ := fs.cipher.DecryptedSize(rf.Int64Size())
	modTime := rf.ModTime()
	fs.storeNode(path, &node{
		fid:               rf.Fid,
		parentFid:         n.parentFid,
		name:              filepath.Base(path),
		size:              decSize,
		encSize:           rf.Int64Size(),
		currentPath:       path,
		isFolder:          rf.IsDir(),
		mtime:             modTime,
		baseServerMtime:   modTime.UnixMilli(),
		baseServerSize:    decSize,
		lastMetadataCheck: time.Now(),
		source:            "remote",
	})
}

// Getattr 拦截元数据请求
func (fs *QryptFS) Getattr(path string, stat *fuse.Stat_t, fh uint64) (errc int) {
	if strings.Contains(path, "/.DS_Store") || strings.Contains(path, "/._") {
		return -fuse.ENOENT
	}
	if isFinderTrashDir(path) {
		fillVirtualTrashStat(stat)
		return 0
	}

	n, errc := fs.lookupExtended(path, false)
	if errc != 0 {
		return errc
	}

	uid, gid, _ := fuse.Getcontext()
	stat.Uid = uid
	stat.Gid = gid

	if n.isFolder {
		stat.Mode = fuse.S_IFDIR | 0755
		stat.Nlink = 2
		// 将文件夹大小设为子项数量，有助于某些工具展示
		n.mu.RLock()
		stat.Size = int64(len(n.children))
		n.mu.RUnlock()
	} else {
		stat.Mode = fuse.S_IFREG | 0644
		stat.Size = n.size
		stat.Nlink = 1
	}
	stat.Mtim = fuse.NewTimespec(n.mtime)
	stat.Atim = stat.Mtim
	stat.Ctim = stat.Mtim
	return 0
}

// Readdir 列出目录内容
func (fs *QryptFS) Readdir(path string, fill func(name string, stat *fuse.Stat_t, ofst int64) bool, ofst int64, fh uint64) (errc int) {
	fill(".", nil, 0)
	fill("..", nil, 0)
	if isFinderTrashDir(path) {
		if filepath.Clean(path) == "/.Trashes" {
			uid, _, _ := fuse.Getcontext()
			stat := &fuse.Stat_t{}
			fillVirtualTrashStat(stat)
			fill(fmt.Sprint(uid), stat, 0)
		}
		return 0
	}

	n, errc := fs.lookup(path)
	if errc != 0 {
		return errc
	}

	n.mu.RLock()
	lastCheck := n.lastMetadataCheck
	childCount := len(n.children)
	parentFid := n.fid // 获取 FID 用于后续无锁调用
	n.mu.RUnlock()

	// 增加防御逻辑：如果子节点为空，且不是刚刚检查过(5s内)，则强制刷新一次
	forceRefresh := childCount == 0 && time.Since(lastCheck) > 5*time.Second

	if time.Since(lastCheck) > MetadataTTL || forceRefresh {
		// --- 修复：在无锁状态下进行网络请求和合并操作 ---
		startFetch := time.Now()
		files, err := fs.fetchFiles(parentFid)
		if err != nil {
			driver.Log.Errorf("[FUSE] Readdir ListFiles failed for %s: %v\n", path, err)
			// 如果获取失败，仍然尝试用本地缓存展示
		} else {
			fetchDuration := time.Since(startFetch)
			startMerge := time.Now()
			fs.MergeRemoteChanges(path, parentFid, files)
			mergeDuration := time.Since(startMerge)

			if fetchDuration > 2*time.Second || mergeDuration > 2*time.Second {
				driver.Log.Infof("[PERF] Readdir slow path %s: fetch=%v, merge=%v, files=%d\n", path, fetchDuration, mergeDuration, len(files))
			}

			n.mu.Lock()
			n.lastMetadataCheck = time.Now()
			n.mu.Unlock()

			// OPTIMIZATION: Background prefetch child directories
			n.mu.RLock()
			var childrenToPrefetch []*node
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

				// 如果子目录正在删除中，不要预取它，否则会把刚删掉的文件又拉回来
				if isDir && childFid != "" && !strings.HasPrefix(childFid, "local_") && time.Since(childLastCheck) > MetadataTTL {
					if _, inDeletion := fs.activeDeletions.Load(childFid); inDeletion {
						continue
					}
					if fs.isUnderDeletingDir(childPath) {
						continue
					}

					go func(fid, cpath string) {
						defer func() {
							if r := recover(); r != nil {
								driver.Log.Errorf("PANIC in background subdir lookup: %v\n%s\n", r, debug.Stack())
							}
						}()
						// 使用信号量限制并发预取
						select {
						case fs.prefetchSem <- struct{}{}:
							defer func() { <-fs.prefetchSem }()
						default:
							// 队列已满，放弃本次预取，优先保证主线程
							return
						}
						// 在后台为子目录触发远程刷新，填充 cache
						prefetchFiles, err := fs.fetchFiles(fid)
						if err != nil {
							return
						}
						// 再次检查，防止在网络请求期间开始了删除
						if fs.isUnderDeletingDir(cpath) {
							return
						}
						fs.MergeRemoteChanges(cpath, fid, prefetchFiles)
						if cpn, ok := fs.fidNodes.Load(fid); ok {
							cn := cpn.(*node)
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

	// 重新获取读锁以进行列表填充
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

// Mkdir 创建文件夹
func (fs *QryptFS) Mkdir(path string, mode uint32) (errc int) {
	driver.Log.Infof("[FUSE] Mkdir: path=%s, mode=%o\n", path, mode)
	if isFinderTrashPath(path) {
		return 0
	}
	parentPath := filepath.Dir(path)
	name := filepath.Base(path)

	parentNode, errc := fs.lookup(parentPath)
	if errc != 0 {
		return errc
	}

	encName := fs.cipher.EncryptSegment(name)

	var logID int64
	if fs.cache != nil {
		if db, ok := fs.cache.GetDB().(*cache.CacheDB); ok {
			payload, _ := json.Marshal(opsPayload{ParentFid: parentNode.fid, Name: encName})
			logID, _ = db.AddOpsLogEntry("MKDIR", "", path, string(payload))
		}
	}

	fid, err := fs.driver.CreateDir(parentNode.fid, encName)
	if err != nil {
		// --- 核心修复：处理“目录已存在”冲突 ---
		if strings.Contains(err.Error(), driver.QuarkErrDirAlreadyExists) {
			driver.Log.Infof("Mkdir: %s already exists on server, attempting to adopt FID\n", path)
			// 尝试找回已存在的 FID
			if foundFid, findErr := fs.driver.FindChildByName(parentNode.fid, encName); findErr == nil {
				fid = foundFid
				// 重要：如果该 FID 正在删除队列中（墓碑），必须立即撤销它
				if _, inDeletion := fs.activeDeletions.Load(fid); inDeletion {
					driver.Log.Infof("Mkdir: revoking tombstone for adopted FID %s\n", fid)
					fs.activeDeletions.Delete(fid)
					// 同时清理父目录索引中的记录
					if val, ok := fs.deletionsByParent.Load(parentNode.fid); ok {
						val.(*sync.Map).Delete(fid)
					}
				}
			} else {
				return -fuse.EIO
			}
		} else {
			return -fuse.EIO
		}
	}

	if logID > 0 && fs.cache != nil {
		if db, ok := fs.cache.GetDB().(*cache.CacheDB); ok {
			_ = db.UpdateOpsLogStatus(logID, "DONE")
		}
	}

	// --- 核心修复：重建目录时，无差别清除该路径下的所有旧状态 ---
	// 1. 清除路径拦截
	fs.deletingPaths.Delete(path)
	prefix := path
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	fs.deletingPaths.Range(func(key, value interface{}) bool {
		if strings.HasPrefix(key.(string), prefix) {
			fs.deletingPaths.Delete(key)
		}
		return true
	})

	// 2. 强力清除该路径及其子路径下的所有墓碑（Tombstones）
	// 不再依赖复杂的 FID 递归，直接遍历 activeDeletions 清理匹配路径的任务
	fs.activeDeletions.Range(func(key, value interface{}) bool {
		state := value.(*deletionState)
		if state.path == path || strings.HasPrefix(state.path, prefix) {
			fs.activeDeletions.Delete(key)
			// 同时清理父级索引
			if val, ok := fs.deletionsByParent.Load(state.parentFid); ok {
				val.(*sync.Map).Delete(key)
			}
		}
		return true
	})
	// ------------------------------------------------------

	fs.storeNode(path, &node{
		fid:               fid,
		parentFid:         parentNode.fid,
		name:              name,
		currentPath:       path,
		isFolder:          true,
		mtime:             time.Now(),
		baseServerMtime:   time.Now().UnixMilli(),
		baseServerSize:    0,
		lastMetadataCheck: time.Now(),
		source:            "local",
	})
	fs.driver.ClearNegativeCache(parentNode.fid, name)
	// 额外清理：如果该路径曾被记录为负缓存，确保彻底清除
	if v, ok := fs.nodes.Load(parentPath); ok {
		fs.driver.ClearNegativeCache(v.(*node).fid, name)
	}
	fs.driver.RemoveDirCache(parentNode.fid)

	return 0
}

// Unlink 删除文件
func (fs *QryptFS) Unlink(path string) (errc int) {
	driver.Log.Infof("[FUSE] Unlink: path=%s\n", path)
	if isFinderTrashPath(path) {
		return 0
	}

	n, errc := fs.lookupExtended(path, false)
	if errc != 0 {
		return errc
	}

	if n.isFolder {
		return -fuse.EISDIR
	}

	if !strings.HasPrefix(n.fid, "local_") {
		// --- 核心改进：检查是否已经在删除队列中，防止重复任务 ---
		if _, exists := fs.activeDeletions.Load(n.fid); exists {
			fs.deleteNodePath(path, n)
			return 0
		}

		// 添加状态化的“墓碑”标记
		fs.activeDeletions.Store(n.fid, &deletionState{
			parentFid: n.parentFid,
			path:      path,
			apiDone:   false, // 初始为 false，等待 worker 完成 API 调用
		})
		// 维护辅助索引
		actualMap, _ := fs.deletionsByParent.LoadOrStore(n.parentFid, &sync.Map{})
		actualMap.(*sync.Map).Store(n.fid, true)

		// 异步处理：先发往 opsLogChan (由后台 worker 批量入库)，再发往 metadataOpChan (执行删除)
		task := metadataTask{
			opType: "DELETE",
			path:   path,
			node:   n,
			fids:   []string{n.fid},
		}
		fs.opsLogChan <- task
		fs.metadataOpChan <- task
	} else {
		// 本地文件，也改为异步清理，释放 FUSE 线程
		fs.metadataOpChan <- metadataTask{
			opType: "LOCAL_CLEANUP",
			path:   path,
			node:   n,
		}
	}

	// 内存删除放在最后，确保墓碑已立好
	fs.deleteNodePath(path, n)

	return 0
}

// isUnderDeletingDir checks if the given path is under a directory currently being deleted
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

// Rmdir 删除文件夹
func (fs *QryptFS) Rmdir(path string) (errc int) {
	driver.Log.Infof("[FUSE] Rmdir: path=%s\n", path)
	if isFinderTrashPath(path) {
		return 0
	}

	n, errc := fs.lookupExtended(path, false)
	if errc != 0 {
		return errc
	}

	if !n.isFolder {
		return -fuse.ENOTDIR
	}

	// --- 标准行为：检查是否为空（忽略正在删除的子项） ---
	n.mu.RLock()
	type childInfo struct {
		fid  string
		path string
		node *node
	}
	var children []childInfo
	for name, child := range n.children {
		child.mu.RLock()
		children = append(children, childInfo{
			fid:  child.fid,
			path: filepath.Join(path, name),
			node: child,
		})
		child.mu.RUnlock()
	}
	n.mu.RUnlock()

	// Mark directory as being deleted to prevent MergeRemoteChanges from re-adding children
	// 必须在 isEmpty 检查之前设置，这样 isUnderDeletingDir 才能检测到嵌套子目录正在被删除
	fs.deletingPaths.Store(path, struct{}{})

	isEmpty := true
	for _, c := range children {
		if c.fid != "" && !strings.HasPrefix(c.fid, "local_") {
			if _, inDeletion := fs.activeDeletions.Load(c.fid); inDeletion {
				continue // 忽略已标记删除的
			}
		}
		// 也要检查路径是否在删除保护中
		if fs.isUnderDeletingDir(c.path) {
			continue
		}
		isEmpty = false
		break
	}

	if !isEmpty {
		fs.deletingPaths.Delete(path) // 清理保护标记，目录实际上没删除
		driver.Log.Infof("Rmdir: %s is not empty in memory, returning ENOTEMPTY\n", path)
		return -fuse.ENOTEMPTY
	}


	if !strings.HasPrefix(n.fid, "local_") {
		// --- 核心改进：检查是否已经在删除队列中 ---
		if _, exists := fs.activeDeletions.Load(n.fid); exists {
			fs.deleteNodePath(path, n)
			return 0
		}

		// 添加状态化的“墓碑”标记
		fs.activeDeletions.Store(n.fid, &deletionState{
			parentFid: n.parentFid,
			path:      path,
			apiDone:   false,
		})
		// 维护辅助索引
		actualMap, _ := fs.deletionsByParent.LoadOrStore(n.parentFid, &sync.Map{})
		actualMap.(*sync.Map).Store(n.fid, true)

		// 异步处理
		task := metadataTask{
			opType: "DELETE",
			path:   path,
			node:   n,
			fids:   []string{n.fid},
		}
		fs.opsLogChan <- task
		fs.metadataOpChan <- task
	} else {
		// 本地目录异步清理
		fs.metadataOpChan <- metadataTask{
			opType: "LOCAL_CLEANUP_DIR",
			path:   path,
			node:   n,
		}
		fs.deletingPaths.Delete(path)
	}

	// 内存删除放在最后，待 MergeRemoteChanges 确认消失后再彻底移除保护
	fs.deleteNodePath(path, n)

	return 0
}

// Rename 重命名或移动文件
func (fs *QryptFS) Rename(oldPath string, newPath string) (errc int) {
	driver.Log.Infof("[FUSE] Rename: oldPath=%s, newPath=%s\n", oldPath, newPath)
	oldNode, errc := fs.lookup(oldPath)
	if errc != 0 {
		return errc
	}
	if isFinderTrashPath(newPath) {
		if oldNode.isFolder {
			return fs.Rmdir(oldPath)
		}
		return fs.Unlink(oldPath)
	}

	oldParent := filepath.Dir(oldPath)
	newParent := filepath.Dir(newPath)
	newName := filepath.Base(newPath)
	isLocal := strings.HasPrefix(oldNode.fid, "local_")

	var logID int64
	if !isLocal && fs.cache != nil {
		if db, ok := fs.cache.GetDB().(*cache.CacheDB); ok {
			var moveFid string
			if oldParent != newParent {
				np, ec := fs.lookup(newParent)
				if ec == 0 {
					moveFid = np.fid
				}
			}
			encName := fs.cipher.EncryptSegment(newName)
			payload, _ := json.Marshal(opsPayload{Fids: []string{oldNode.fid}, ParentFid: moveFid, CurrentDirFid: oldNode.parentFid, Name: encName})
			logID, _ = db.AddOpsLogEntry("RENAME", oldPath, newPath, string(payload))
		}
	}

	if oldParent != newParent {
		newParentNode, errc := fs.lookup(newParent)
		if errc != 0 {
			return errc
		}

		if !isLocal {
			// Retry Move to handle transient Quark API errors (e.g. 23008 conflict)
			var moveErr error
			for attempt := 0; attempt < 5; attempt++ {
				moveErr = fs.driver.Move([]string{oldNode.fid}, newParentNode.fid, oldNode.parentFid)
				if moveErr == nil {
					break
				}
				if strings.Contains(moveErr.Error(), driver.QuarkErrDirAlreadyExists) || strings.Contains(moveErr.Error(), "conflict") {
					driver.Log.Errorf("Rename Move: transient error on attempt %d for %s: %v\n", attempt+1, oldPath, moveErr)
					time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
					fs.driver.RemoveDirCache(newParentNode.fid)
					continue
				}
				break // non-retryable error
			}
			if moveErr != nil {
				driver.Log.Errorf("Rename Move failed for %s -> %s: %v\n", oldPath, newPath, moveErr)
				// 夸克网盘 API 限制：不能移动到子目录
				if strings.Contains(moveErr.Error(), "23017") || strings.Contains(moveErr.Error(), "subdirs") {
					driver.Log.Info("Rename: Quark API does not allow moving files into subdirectories\n")
				}
				return -fuse.EIO
			}
		}

		oldNode.mu.Lock()
		oldNode.parentFid = newParentNode.fid
		oldNode.mu.Unlock()

		oldParentNode, errc := fs.lookup(oldParent)
		if errc == 0 {
			fs.driver.RemoveDirCache(oldParentNode.fid)
		}
		fs.driver.RemoveDirCache(newParentNode.fid)
	}

	if !isLocal {
		encName := fs.cipher.EncryptSegment(newName)
		// OPTIMIZATION + BUGFIX: Skip Rename if encrypted name is unchanged.
		// EncryptSegment is deterministic, so same plaintext → same ciphertext.
		// This avoids a redundant API call and prevents Quark 23008 conflicts
		// when Move already set the file's name (e.g. cross-directory move with same name).
		oldEncName := fs.cipher.EncryptSegment(oldNode.name)
		if encName != oldEncName {
			// Retry Rename to handle transient Quark API errors
			var renameErr error
			for attempt := 0; attempt < 5; attempt++ {
				renameErr = fs.driver.Rename(oldNode.fid, encName)
				if renameErr == nil {
					break
				}
				if strings.Contains(renameErr.Error(), driver.QuarkErrDirAlreadyExists) || strings.Contains(renameErr.Error(), "conflict") {
					driver.Log.Errorf("Rename: transient error on attempt %d for %s: %v\n", attempt+1, oldPath, renameErr)
					time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
					continue
				}
				break
			}
			if renameErr != nil {
				driver.Log.Errorf("Rename failed for %s -> %s: %v\n", oldPath, newPath, renameErr)
				return -fuse.EIO
			}
		} else {
			driver.Log.Infof("Rename: skipping Rename API call, name unchanged (%s)\n", newName)
		}
	}

	if logID > 0 && fs.cache != nil {
		if db, ok := fs.cache.GetDB().(*cache.CacheDB); ok {
			_ = db.UpdateOpsLogStatus(logID, "DONE")
		}
	}

	oldNode.mu.Lock()
	oldNode.name = newName
	oldNode.mu.Unlock()

	fs.replaceNodePath(oldPath, newPath, oldNode)
	fs.persistPendingPath(oldPath, newPath, oldNode)

	if oldNode.isFolder {
		fs.renameSubtreePaths(oldPath, newPath)
	}

	parentNode, errc := fs.lookup(newParent)
	if errc == 0 {
		fs.driver.ClearNegativeCache(parentNode.fid, newName)
		fs.driver.RemoveDirCache(parentNode.fid)
	}
	if oldParent != newParent {
		oldParentNode, errc := fs.lookup(oldParent)
		if errc == 0 {
			fs.driver.RemoveDirCache(oldParentNode.fid)
		}
	}

	return 0
}

// Open 打开文件
func (fs *QryptFS) Open(path string, flags int) (errc int, fh uint64) {
	n, errc := fs.lookup(path)
	if errc != 0 {
		return errc, 0
	}
	return 0, uint64(uintptr(unsafe.Pointer(n)))
}

// Access 访问检查（在 defer_permissions 下仍提供显式允许，避免 ENOSYS 被解释为权限错误）
func (fs *QryptFS) Access(path string, mask uint32) (errc int) {
	if strings.Contains(path, "/.DS_Store") || strings.Contains(path, "/._") {
		return -fuse.ENOENT
	}
	if isFinderTrashDir(path) {
		return 0
	}
	_, errc = fs.lookup(path)
	if errc != 0 {
		return errc
	}
	return 0
}

// fetchFiles gated version of ListFiles to deduplicate concurrent requests
func (fs *QryptFS) fetchFiles(fid string) ([]driver.File, error) {
	if fid == "" {
		return nil, nil
	}

	// 1. 尝试从同步锁中获取
	waitChan, loading := fs.fetching.LoadOrStore(fid, make(chan struct{}))
	if loading {
		// 已经有协程在拉取了，等待它完成，增加超时保护
		select {
		case <-waitChan.(chan struct{}):
			// 完成后，驱动层缓存应该是最新的，直接拉取驱动缓存
			return fs.driver.ListFiles(fid)
		case <-time.After(30 * time.Second):
			return nil, fmt.Errorf("fetchFiles timeout waiting for concurrent fetch")
		}
	}

	// 2. 我是第一个拉取的，负责执行并通知其他协程
	defer func() {
		close(waitChan.(chan struct{}))
		fs.fetching.Delete(fid)
	}()

	return fs.driver.ListFiles(fid)
}

// maybeSavePendingNodeLocked saves the node state to the persistent database if needed.
// Must be called with node.mu locked.
func (fs *QryptFS) maybeSavePendingNodeLocked(path string, n *node, force bool) error {
	if fs.cache == nil {
		return nil
	}
	
	now := time.Now()
	if !force && now.Sub(n.lastPendingSave) < pendingNodeSaveInterval && 
	   (n.size - n.lastPendingSize) < pendingNodeSaveSizeStep {
		return nil
	}

	err := fs.cache.SavePendingNode(path, n.fid, n.parentFid, n.name, n.localPath, n.size, n.isFolder, n.fileNonce[:], n.baseServerMtime, n.baseServerSize)
	if err == nil {
		n.lastPendingSave = now
		n.lastPendingSize = n.size
	}
	return err
}
