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
	n.mu.RUnlock()
	if path != "" {
		return path
	}

	fs.nodes.Range(func(key, value interface{}) bool {
		if value == n {
			path = key.(string)
			return false
		}
		return true
	})
	if path != "" {
		n.mu.Lock()
		n.currentPath = path
		n.mu.Unlock()
	}
	return path
}

func (fs *QryptFS) storeNode(path string, n *node) {
	n.mu.Lock()
	n.currentPath = path
	n.mu.Unlock()
	fs.nodes.Store(path, n)
}

func (fs *QryptFS) replaceNodePath(oldPath, newPath string, n *node) {
	if oldPath != newPath {
		fs.nodes.Delete(oldPath)
	}
	n.mu.Lock()
	n.currentPath = newPath
	n.mu.Unlock()
	fs.nodes.Store(newPath, n)
}

func (fs *QryptFS) deleteNodePath(path string, n *node) {
	fs.nodes.Delete(path)
	if n == nil {
		return
	}
	n.mu.Lock()
	if n.currentPath == path {
		n.currentPath = ""
	}
	n.mu.Unlock()
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
	oldPrefix := oldPath
	if !strings.HasSuffix(oldPrefix, "/") {
		oldPrefix += "/"
	}
	newPrefix := newPath
	if !strings.HasSuffix(newPrefix, "/") {
		newPrefix += "/"
	}

	type renameEntry struct {
		oldPath string
		newPath string
		node    *node
	}

	var entries []renameEntry
	fs.nodes.Range(func(key, value interface{}) bool {
		path, ok := key.(string)
		if !ok || !strings.HasPrefix(path, oldPrefix) {
			return true
		}
		entries = append(entries, renameEntry{
			oldPath: path,
			newPath: newPrefix + strings.TrimPrefix(path, oldPrefix),
			node:    value.(*node),
		})
		return true
	})

	for _, entry := range entries {
		fs.replaceNodePath(entry.oldPath, entry.newPath, entry.node)
		fs.persistPendingPath(entry.oldPath, entry.newPath, entry.node)
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

		// For synced nodes (non-local fid), verify the file still exists on server.
		// If TTL expired, we refresh metadata.
		if !strings.HasPrefix(fid, "local_") {
			if !isFolder && !isDirty && time.Since(lastCheck) > MetadataTTL {
				// Refresh single file metadata
				files, err := fs.driver.ListFiles(parentFid)
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
							break
						}
					}
				}
			}

			if path != "/" && !fs.fileExistsOnServer(fid, parentFid) {
				driver.Log.Printf("lookup: file %s (fid=%s) not found on server, using cached node\n", path, fid)
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
				lastMetadataCheck: time.Now(),
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
	Fid       string   `json:"fid,omitempty"`
	Fids      []string `json:"fids,omitempty"`
	ParentFid string   `json:"parent_fid,omitempty"`
	Name      string   `json:"name,omitempty"`
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
				_ = fs.driver.Move(p.Fids, p.ParentFid)
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
		decName, _ := fs.cipher.DecryptSegment(f.FileName)
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
	fs.nodes.Range(func(key, value interface{}) bool {
		p := key.(string)
		if p == parentPath {
			return true
		}
		if strings.HasPrefix(p, prefix) {
			rel := strings.TrimPrefix(p, prefix)
			if !strings.Contains(rel, "/") {
				localEntries = append(localEntries, nodeEntry{path: p, node: value.(*node)})
			}
		}
		return true
	})

	seenLocalNames := make(map[string]bool)
	for _, entry := range localEntries {
		n := entry.node
		n.mu.RLock()
		name := n.name
		fid := n.fid
		isDirty := n.isDirty
		baseMtime := n.baseServerMtime
		n.mu.RUnlock()

		seenLocalNames[name] = true

		rf, exists := remoteMap[name]
		if !exists {
			// 远端已删除
			if !strings.HasPrefix(fid, "local_") {
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
					n.mu.Unlock()
				}
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
			if remoteMtime > baseMtime {
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
	n.mu.RUnlock()

	var files []driver.File
	var err error

	if time.Since(lastCheck) > MetadataTTL {
		files, err = fs.driver.ListFiles(n.fid)
		if err != nil {
			return -fuse.EIO
		}
		fs.MergeRemoteChanges(path, n.fid, files)
		n.mu.Lock()
		n.lastMetadataCheck = time.Now()
		n.mu.Unlock()
	} else {
		files, err = fs.driver.ListFiles(n.fid)
		if err != nil {
			return -fuse.EIO
		}
	}

	uid, gid, _ := fuse.Getcontext()

	seen := make(map[string]bool)
	for _, f := range files {
		decName := ""
		if fs.cache != nil {
			if cached, ok, cerr := fs.cache.GetCachedName(f.Fid, f.FileName); cerr == nil && ok {
				decName = cached
			}
		}
		if decName == "" {
			decName, err = fs.cipher.DecryptSegment(f.FileName)
			if err != nil {
				driver.Log.Printf("[FUSE] DecryptSegment failed for '%s': %v\n", f.FileName, err)
				continue
			}
			if fs.cache != nil {
				_ = fs.cache.SaveCachedName(f.Fid, f.FileName, decName)
			}
		}

		stat := &fuse.Stat_t{}
		stat.Uid = uid
		stat.Gid = gid

		decSize, _ := fs.cipher.DecryptedSize(f.Int64Size())
		modTime := f.ModTime()
		if f.IsDir() {
			stat.Mode = fuse.S_IFDIR | 0777
		} else {
			stat.Mode = fuse.S_IFREG | 0666
			stat.Size = decSize
		}
		stat.Mtim = fuse.NewTimespec(modTime)
		stat.Atim = stat.Mtim
		stat.Ctim = stat.Mtim

		childPath := path
		if !strings.HasSuffix(childPath, "/") {
			childPath += "/"
		}
		childPath += decName

		// 保护逻辑：如果本地已有该路径的 Dirty 节点，不覆盖它
		skipStore := false
		if v, ok := fs.nodes.Load(childPath); ok {
			existing := v.(*node)
			existing.mu.RLock()
			if existing.isDirty {
				skipStore = true
			}
			existing.mu.RUnlock()
		}

		if !skipStore {
			fs.storeNode(childPath, &node{
				fid:         f.Fid,
				parentFid:   n.fid,
				name:        decName,
				size:        decSize,
				currentPath: childPath,
				isFolder:    f.IsDir(),
				mtime:       modTime,
			})
		}

		seen[decName] = true
		fill(decName, stat, 0)
	}

	// 补充本地存在但服务器上尚未出现的节点（例如正在同步中的新文件）
	prefix := path
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	fs.nodes.Range(func(key, value interface{}) bool {
		childPath, ok := key.(string)
		if !ok || !strings.HasPrefix(childPath, prefix) || childPath == prefix {
			return true
		}

		relPath := strings.TrimPrefix(childPath, prefix)
		if strings.Contains(relPath, "/") {
			return true // 深度超过一级
		}

		if seen[relPath] {
			return true
		}

		childNode := value.(*node)
		stat := &fuse.Stat_t{}
		stat.Uid = uid
		stat.Gid = gid

		childNode.mu.RLock()
		if childNode.isFolder {
			stat.Mode = fuse.S_IFDIR | 0777
		} else {
			stat.Mode = fuse.S_IFREG | 0666
			stat.Size = childNode.size
		}
		stat.Mtim = fuse.NewTimespec(childNode.mtime)
		childNode.mu.RUnlock()
		stat.Atim = stat.Mtim
		stat.Ctim = stat.Mtim

		fill(relPath, stat, 0)
		return true
	})

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
			payload, _ := json.Marshal(opsPayload{Fids: []string{oldNode.fid}, ParentFid: moveFid, Name: encName})
			logID, _ = db.AddOpsLogEntry("RENAME", oldPath, newPath, string(payload))
		}
	}

	if oldParent != newParent {
		newParentNode, errc := fs.lookup(newParent)
		if errc != 0 {
			return errc
		}

		if !isLocal {
			err := fs.driver.Move([]string{oldNode.fid}, newParentNode.fid)
			if err != nil {
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
		err := fs.driver.Rename(oldNode.fid, encName)
		if err != nil {
			return -fuse.EIO
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
