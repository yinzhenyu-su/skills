package vfs

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/winfsp/cgofuse/fuse"
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
	n.mu.RUnlock()
	if !shouldSave {
		return
	}

	_ = fs.cache.SavePendingNode(newPath, fid, parentFid, name, localPath, size, false, nonce)
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
		// For synced nodes (non-local fid), verify the file still exists on server.
		// If not found, we still return the cached node rather than deleting it,
		// to handle cases where server hasn't propagated the fid yet.
		if !strings.HasPrefix(n.fid, "local_") {
			if !fs.fileExistsOnServer(n.fid, n.parentFid) {
				driver.Log.Printf("lookup: file %s (fid=%s) not found on server, using cached node\n", path, n.fid)
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
				fid:         f.Fid,
				parentFid:   currentFid,
				name:        decName,
				size:        decSize,
				encSize:     f.Int64Size(),
				currentPath: currentPath,
				isFolder:    f.IsDir(),
				mtime:       modTime,
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

	files, err := fs.driver.ListFiles(n.fid)
	if err != nil {
		return -fuse.EIO
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
	fid, err := fs.driver.CreateDir(parentNode.fid, encName)
	if err != nil {
		return -fuse.EIO
	}

	fs.storeNode(path, &node{
		fid:         fid,
		parentFid:   parentNode.fid,
		name:        name,
		currentPath: path,
		isFolder:    true,
		mtime:       time.Now(),
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
		err := fs.driver.Delete([]string{n.fid})
		if err != nil {
			driver.Log.Printf("Unlink failed for %s (fid=%s): %v\n", path, n.fid, err)
			return -fuse.EIO
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
		err := fs.driver.Delete([]string{n.fid})
		if err != nil {
			driver.Log.Printf("Rmdir failed for %s (fid=%s): %v\n", path, n.fid, err)
			return -fuse.EIO
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
