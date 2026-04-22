package vfs

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
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
			p.children[filepath.Base(path)] = n
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
	fs.nodes.Delete(path)

	// 从父节点移除引用
	if path != "/" {
		parentPath := filepath.Dir(path)
		if v, ok := fs.nodes.Load(parentPath); ok {
			p := v.(*node)
			p.mu.Lock()
			if p.children != nil {
				delete(p.children, filepath.Base(path))
			}
			p.mu.Unlock()
		}
	}

	if n == nil {
		return
	}
	n.mu.Lock()
	fid := n.fid
	if n.currentPath == path {
		n.currentPath = ""
	}
	n.mu.Unlock()

	if fid != "" && !strings.HasPrefix(fid, "local_") {
		fs.fidNodes.Delete(fid)
	}
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
	if v, ok := fs.nodes.Load(path); ok {
		n := v.(*node)

		n.mu.RLock()
		isFolder := n.isFolder
		lastCheck := n.lastMetadataCheck
		isDirty := n.isDirty
		fid := n.fid
		parentFid := n.parentFid
		n.mu.RUnlock()

		// For synced nodes (non-local fid):
		// - If within TTL, trust cached metadata (no API call).
		// - If TTL expired, refresh metadata via ListFiles (which also verifies existence).
		// This eliminates redundant fileExistsOnServer calls on every lookup.
		if !strings.HasPrefix(fid, "local_") {
			if !isFolder && !isDirty && time.Since(lastCheck) > MetadataTTL {
				// Refresh single file metadata + verify existence in one ListFiles call
				files, err := fs.driver.ListFiles(parentFid)
				found := false
				if err == nil {
					for _, f := range files {
						if f.Fid == fid {
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
					driver.Log.Printf("lookup: file %s (fid=%s) deleted from server, removing from cache\n", path, fid)
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

		// 否则，列出父目录内容来寻找
		files, err := fs.driver.ListFiles(currentFid)
		if err != nil {
			return nil, -fuse.EIO
		}

		found := false
		for _, f := range files {
			decName, _ := fs.cipher.DecryptSegment(f.FileName)
			if decName != part {
				continue
			}

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
			driver.Log.Printf("[FUSE] lookup: creating node for '%s' (FID='%s') with parentFid='%s'\n", decName, f.Fid, currentFid)
			lastCheck := time.Time{}
			if !f.IsDir() {
				lastCheck = time.Now()
			}
			n := &node{
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
			}
			fs.storeNode(currentPath, n)
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
		driver.Log.Printf("recoverPendingOps: failed to get logs: %v\n", err)
		return
	}

	for _, l := range logs {
		driver.Log.Printf("recoverPendingOps: retrying %s (id=%d) %s -> %s\n", l.OpType, l.ID, l.SourcePath, l.TargetPath)
		var p opsPayload
		_ = json.Unmarshal([]byte(l.Payload), &p)

		var err error
		switch l.OpType {
		case "MKDIR":
			_, err = fs.driver.CreateDir(p.ParentFid, p.Name)
		case "UNLINK", "RMDIR":
			err = fs.driver.Delete(p.Fids)
		case "RENAME":
			if p.ParentFid != "" {
				_ = fs.driver.Move(p.Fids, p.ParentFid, p.CurrentDirFid)
			}
			err = fs.driver.Rename(p.Fids[0], p.Name)
		}

		if err == nil {
			driver.Log.Printf("recoverPendingOps: retry %d (%s) SUCCEEDED\n", l.ID, l.OpType)
			_ = db.UpdateOpsLogStatus(l.ID, "DONE")
		} else {
			driver.Log.Printf("recoverPendingOps: retry %d failed: %v\n", l.ID, err)
		}
	}
}

func (fs *QryptFS) MergeRemoteChanges(parentPath string, parentFid string, remoteFiles []driver.File) {
	seenFids := make(map[string]bool)
	remoteMap := make(map[string]driver.File)

	for _, f := range remoteFiles {
		seenFids[f.Fid] = true
		
		decName := ""
		// 1. 优先使用缓存中已知的解密名称
		if v, ok := fs.fidNodes.Load(f.Fid); ok {
			pn := v.(*node)
			pn.mu.RLock()
			decName = pn.name
			pn.mu.RUnlock()
		}
		// 2. 如果未知，则进行昂贵的解密
		if decName == "" {
			decName, _ = fs.cipher.DecryptSegment(f.FileName)
		}
		
		// 跳过同名文件（夸克网盘允许 xxx 和 xxx(1) 共存，解密后可能重名）
		if _, exists := remoteMap[decName]; exists {
			driver.Log.Printf("MergeRemoteChanges: skipping duplicate remote file '%s' (fid=%s) in %s\n", decName, f.Fid, parentPath)
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
	for _, entry := range localEntries {
		n := entry.node
		n.mu.RLock()
		name := n.name
		fid := n.fid
		isDirty := n.isDirty
		baseMtime := n.baseServerMtime
		syncQueued := n.syncQueued
		lastUpload := n.lastUploadTime
		n.mu.RUnlock()

		seenLocalNames[name] = true

		// 跳过正在同步的文件
		if syncQueued {
			driver.Log.Printf("MergeRemoteChanges: skipping %s (sync in progress)\n", entry.path)
			continue
		}
		// 刚上传完成的文件（30秒内），跳过以防 API 索引延迟导致误判为"远程删除"
		// 夸克 API 索引新文件可能需要几秒到几十秒
		if !isDirty && !lastUpload.IsZero() && time.Since(lastUpload) < 30*time.Second {
			driver.Log.Printf("MergeRemoteChanges: skipping %s (just uploaded %dms ago)\n", entry.path, time.Since(lastUpload).Milliseconds())
			continue
		}

		rf, exists := remoteMap[name]
		if !exists {
			// 远端已删除
			if n.source == "local" {
				// 本地新建的文件，远程不可见，跳过删除
				driver.Log.Printf("MergeRemoteChanges: skipping delete for local-only file %s\n", entry.path)
				continue
			}
			if n.source == "merged" {
				// 远程删了，但我本地改过 → 冲突：转为 local 重新上传
				driver.Log.Printf("MergeRemoteChanges: CONFLICT (remote deleted, local merged) for %s. Keeping local.\n", entry.path)
				n.mu.Lock()
				if !strings.HasPrefix(n.fid, "local_") {
					n.fid = "local_" + n.name + "_" + fmt.Sprint(time.Now().UnixNano())
				}
				n.source = "merged"
				n.mu.Unlock()
				continue
			}
			// source == "remote" or unknown
			if !isDirty {
				driver.Log.Printf("MergeRemoteChanges: remote deleted %s, removing local node\n", entry.path)
				fs.deleteNodePath(entry.path, n)
			} else {
				// 冲突：远端删了，但我本地改了。将 fid 转为 local_ 保证继续上传为新文件
				driver.Log.Printf("MergeRemoteChanges: CONFLICT (remote deleted, local dirty) for %s. Turning into local node.\n", entry.path)
				n.mu.Lock()
				if !strings.HasPrefix(n.fid, "local_") {
					n.fid = "local_" + n.name + "_" + fmt.Sprint(time.Now().UnixNano())
				}
				n.source = "merged"
				n.mu.Unlock()
			}
			continue
		}

		// 远端存在
		if !strings.HasPrefix(fid, "local_") {
			if rf.Fid != fid {
				// FID 变了（可能是删了重建），按更新处理
				driver.Log.Printf("MergeRemoteChanges: FID changed for %s (%s -> %s)\n", entry.path, fid, rf.Fid)
			}

			remoteMtime := rf.ModTime().UnixMilli()
			if remoteMtime > baseMtime+2000 {
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
					driver.Log.Printf("MergeRemoteChanges: updated %s from server\n", entry.path)
				} else {
					// 冲突：双向改
					driver.Log.Printf("MergeRemoteChanges: CONFLICT (both modified) for %s. Triggering side-by-side rename.\n", entry.path)
					fs.resolveConflict(entry.path, n, rf)
				}
			}
		} else {
			// 本地是 local_，但远端出现了同名文件（可能是别人上传了同名文件）
			// 但如果刚上传过（30s 内），远程文件很可能是我们自己的上传，
			// 因为 syncFile 在 API 未索引时将 fid 转回了 local_。
			if !lastUpload.IsZero() && time.Since(lastUpload) < 30*time.Second {
				driver.Log.Printf("MergeRemoteChanges: skipping conflict for %s (just uploaded %dms ago, remote file likely ours)\n", entry.path, time.Since(lastUpload).Milliseconds())
				continue
			}
			driver.Log.Printf("MergeRemoteChanges: CONFLICT (local new, remote exists) for %s. Triggering side-by-side rename.\n", entry.path)
			fs.resolveConflict(entry.path, n, rf)
		}
	}

	// 2. 处理远端有但本地没有的新文件
	for name, rf := range remoteMap {
		if seenLocalNames[name] {
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
		driver.Log.Printf("MergeRemoteChanges: added new remote file %s\n", childPath)
	}
}

func (fs *QryptFS) resolveConflict(path string, n *node, rf driver.File) {
	// 1. 重命名本地脏节点
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	conflictPath := fmt.Sprintf("%s [Local Conflict %s]%s", base, time.Now().Format("20060102_150405"), ext)

	driver.Log.Printf("resolveConflict: Renaming local %s -> %s\n", path, conflictPath)

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

	n, errc := fs.lookup(path)
	if errc != 0 {
		return errc
	}

	uid, gid, _ := fuse.Getcontext()
	stat.Uid = uid
	stat.Gid = gid

	if n.isFolder {
		stat.Mode = fuse.S_IFDIR | 0755
		stat.Nlink = 2
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
	n.mu.RUnlock()

	// 增加防御逻辑：如果子节点为空，且不是刚刚检查过(1s内)，则强制刷新一次
	forceRefresh := childCount == 0 && time.Since(lastCheck) > 1*time.Second

	if time.Since(lastCheck) > MetadataTTL || forceRefresh {
		files, err := fs.driver.ListFiles(n.fid)
		if err != nil {
			driver.Log.Printf("[FUSE] Readdir ListFiles failed for %s: %v\n", path, err)
			// 如果获取失败，仍然尝试用本地缓存展示
		} else {
			fs.MergeRemoteChanges(path, n.fid, files)
			n.mu.Lock()
			n.lastMetadataCheck = time.Now()
			n.mu.Unlock()

			// OPTIMIZATION: Background prefetch child directories
			n.mu.RLock()
			for _, child := range n.children {
				child.mu.RLock()
				isDir := child.isFolder
				childFid := child.fid
				childLastCheck := child.lastMetadataCheck
				child.mu.RUnlock()
				if isDir && time.Since(childLastCheck) > MetadataTTL {
					go func(fid string) {
						prefetchFiles, err := fs.driver.ListFiles(fid)
						if err != nil {
							return
						}
						if cpn, ok := fs.fidNodes.Load(fid); ok {
							fs.MergeRemoteChanges(cpn.(*node).currentPath, fid, prefetchFiles)
							cpn.(*node).mu.Lock()
							cpn.(*node).lastMetadataCheck = time.Now()
							cpn.(*node).mu.Unlock()
						}
					}(childFid)
				}
			}
			n.mu.RUnlock()
		}
	}

	uid, gid, _ := fuse.Getcontext()

	// 使用父子引用，避免 O(N) 扫描，且能够展示本地尚未同步的文件
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
	driver.Log.Printf("[FUSE] Mkdir: path=%s, mode=%o\n", path, mode)
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
		return -fuse.EIO
	}

	if logID > 0 && fs.cache != nil {
		if db, ok := fs.cache.GetDB().(*cache.CacheDB); ok {
			_ = db.UpdateOpsLogStatus(logID, "DONE")
		}
	}

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
	fs.driver.RemoveDirCache(parentNode.fid)

	return 0
}

// Unlink 删除文件
func (fs *QryptFS) Unlink(path string) (errc int) {
	driver.Log.Printf("[FUSE] Unlink: path=%s\n", path)
	if isFinderTrashPath(path) {
		return 0
	}
	n, errc := fs.lookup(path)
	if errc != 0 {
		return errc
	}

	if n.isFolder {
		return -fuse.EISDIR
	}

	if !strings.HasPrefix(n.fid, "local_") {
		var logID int64
		if fs.cache != nil {
			if db, ok := fs.cache.GetDB().(*cache.CacheDB); ok {
				payload, _ := json.Marshal(opsPayload{Fids: []string{n.fid}})
				logID, _ = db.AddOpsLogEntry("UNLINK", path, "", string(payload))
			}
		}

		err := fs.driver.Delete([]string{n.fid})
		if err != nil {
			driver.Log.Printf("Unlink failed for %s (fid=%s): %v\n", path, n.fid, err)
			return -fuse.EIO
		}

		if logID > 0 && fs.cache != nil {
			if db, ok := fs.cache.GetDB().(*cache.CacheDB); ok {
				_ = db.UpdateOpsLogStatus(logID, "DONE")
			}
		}
	}

	fs.cleanupLocalUploadState(path, n, false)
	fs.driver.RemoveDirCache(n.parentFid)
	return 0
}

// Rmdir 删除文件夹
func (fs *QryptFS) Rmdir(path string) (errc int) {
	driver.Log.Printf("[FUSE] Rmdir: path=%s\n", path)
	if isFinderTrashDir(path) {
		return 0
	}
	n, errc := fs.lookup(path)
	if errc != 0 {
		return errc
	}

	if !n.isFolder {
		return -fuse.ENOTDIR
	}

	if !strings.HasPrefix(n.fid, "local_") {
		var logID int64
		if fs.cache != nil {
			if db, ok := fs.cache.GetDB().(*cache.CacheDB); ok {
				payload, _ := json.Marshal(opsPayload{Fids: []string{n.fid}})
				logID, _ = db.AddOpsLogEntry("RMDIR", path, "", string(payload))
			}
		}

		err := fs.driver.Delete([]string{n.fid})
		if err != nil {
			driver.Log.Printf("Rmdir failed for %s (fid=%s): %v\n", path, n.fid, err)
			return -fuse.EIO
		}

		if logID > 0 && fs.cache != nil {
			if db, ok := fs.cache.GetDB().(*cache.CacheDB); ok {
				_ = db.UpdateOpsLogStatus(logID, "DONE")
			}
		}
	}

	fs.cleanupLocalUploadState(path, n, true)
	fs.driver.RemoveDirCache(n.parentFid)
	return 0
}

// Rename 重命名或移动文件
func (fs *QryptFS) Rename(oldPath string, newPath string) (errc int) {
	driver.Log.Printf("[FUSE] Rename: oldPath=%s, newPath=%s\n", oldPath, newPath)
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
				if strings.Contains(moveErr.Error(), "23008") || strings.Contains(moveErr.Error(), "conflict") {
					driver.Log.Printf("Rename Move: transient error on attempt %d for %s: %v\n", attempt+1, oldPath, moveErr)
					time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
					fs.driver.RemoveDirCache(newParentNode.fid)
					continue
				}
				break // non-retryable error
			}
			if moveErr != nil {
				driver.Log.Printf("Rename Move failed for %s -> %s: %v\n", oldPath, newPath, moveErr)
				// 夸克网盘 API 限制：不能移动到子目录
				if strings.Contains(moveErr.Error(), "23017") || strings.Contains(moveErr.Error(), "subdirs") {
					driver.Log.Printf("Rename: Quark API does not allow moving files into subdirectories\n")
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
				if strings.Contains(renameErr.Error(), "23008") || strings.Contains(renameErr.Error(), "conflict") {
					driver.Log.Printf("Rename: transient error on attempt %d for %s: %v\n", attempt+1, oldPath, renameErr)
					time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
					continue
				}
				break
			}
			if renameErr != nil {
				driver.Log.Printf("Rename failed for %s -> %s: %v\n", oldPath, newPath, renameErr)
				return -fuse.EIO
			}
		} else {
			driver.Log.Printf("Rename: skipping Rename API call, name unchanged (%s)\n", newName)
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
