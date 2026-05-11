package vfs

import (
	"fmt"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
	"unsafe"

	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/driver"
)

// Create 创建新文件
func (fs *QryptFS) Create(path string, flags int, mode uint32) (errc int, fh uint64) {
	defer func() {
		if r := recover(); r != nil {
			driver.Log.Errorf("PANIC in Create(%s): %v\n%s\n", path, r, debug.Stack())
			errc = -fuse.EIO
		}
	}()
	if fs.isShuttingDown() {
		driver.Log.Warnf("[SHUTDOWN] Rejecting Create: %s\n", path)
		return -fuse.EIO, 0
	}
	driver.Log.Infof("[FUSE] Create: path=%s, flags=%d, mode=%o\n", path, flags, mode)
	if strings.Contains(path, "/.DS_Store") || strings.Contains(path, "/._") {
		return -fuse.ENOENT, 0
	}
	parentPath := filepath.Dir(path)
	parentNode, errc := fs.lookup(parentPath)
	if errc != 0 {
		return errc, 0
	}

	name := filepath.Base(path)
	n := &node{
		fid:               "local_" + name + "_" + fmt.Sprint(time.Now().UnixNano()),
		parentFid:         parentNode.fid,
		name:              name,
		currentPath:       path,
		isFolder:          false,
		mtime:             time.Now(),
		isDirty:           true,
		baseServerMtime:   0, // New local file has no base mtime
		baseServerSize:    0,
		lastMetadataCheck: time.Now(),
		source:            "local",
	}

	nonce, err := fs.cipher.GenerateRandomNonce()
	if err == nil {
		n.fileNonce = nonce
		n.hasNonce = true
	}
	if fs.staging == nil {
		return -fuse.EIO, 0
	}
	localPath, err := fs.staging.Create(n.fid)
	if err != nil {
		return -fuse.EIO, 0
	}
	n.localPath = localPath

	fs.storeNode(path, n)
	fs.driver.ClearNegativeCache(parentNode.fid, name)
	n.mu.Lock()
	if err := fs.maybeSavePendingNodeLocked(path, n, true); err != nil {
		driver.Log.Warnf("Warning: failed to save pending node after Create for %s: %v\n", path, err)
	}
	n.mu.Unlock()
	return 0, uint64(uintptr(unsafe.Pointer(n)))
}

// Mknod 部分 macOS 写入路径会触发 Mknod，转发到 Create 统一处理。
func (fs *QryptFS) Mknod(path string, mode uint32, dev uint64) (errc int) {
	err, _ := fs.Create(path, 0, mode)
	return err
}

// Write 写入文件内容
func (fs *QryptFS) Write(path string, buff []byte, ofst int64, fh uint64) (n int) {
	defer func() {
		if r := recover(); r != nil {
			driver.Log.Errorf("PANIC in Write(%s): %v\n%s\n", path, r, debug.Stack())
			n = 0
		}
	}()
	if fs.isShuttingDown() {
		driver.Log.Warnf("[SHUTDOWN] Rejecting Write: %s\n", path)
		return 0
	}
	driver.Log.Infof("[FUSE] Write: path=%s, len=%d, offset=%d, fh=%d\n", path, len(buff), ofst, fh)
	if strings.Contains(path, "/.DS_Store") || strings.Contains(path, "/._") {
		return 0
	}
	node, errc := fs.lookupExtended(path, false)
	if errc != 0 {
		return 0
	}

	node.mu.Lock()

	if fs.staging == nil {
		node.mu.Unlock()
		return 0
	}
	if node.localPath == "" {
		// Node was previously synced (fid is now remote). Generate a new local fid for the new staging file.
		newFid := "local_" + node.name + "_" + fmt.Sprint(time.Now().UnixNano())
		localPath, err := fs.staging.Create(newFid)
		if err != nil {
			driver.Log.Warnf("Warning: failed to create staging file for Write on %s: %v\n", path, err)
			node.mu.Unlock()
			return 0
		}
		node.localPath = localPath
	}

	node.isDirty = true
	node.mtime = time.Now()
	written, err := fs.staging.WriteAt(node.localPath, buff, ofst)
	if err != nil {
		node.mu.Unlock()
		return 0
	}
	if ofst+int64(written) > node.size {
		node.size = ofst + int64(written)
	}

	if err := fs.maybeSavePendingNodeLocked(path, node, false); err != nil {
		driver.Log.Warnf("Warning: failed to save pending node after Write for %s: %v\n", path, err)
	}

	// 不在此处触发 sync —— 等 Release（文件关闭）时再一次性上传。
	// 这样一个文件只上传一次，避免 mtime 竞态导致的重复文件。
	node.mu.Unlock()
	return written
}

// Truncate 调整文件大小（用于 cp/touch 等写入前截断流程）
func (fs *QryptFS) Truncate(path string, size int64, fh uint64) (errc int) {
	defer func() {
		if r := recover(); r != nil {
			driver.Log.Errorf("PANIC in Truncate(%s): %v\n%s\n", path, r, debug.Stack())
			errc = -fuse.EIO
		}
	}()
	if fs.isShuttingDown() {
		driver.Log.Warnf("[SHUTDOWN] Rejecting Truncate: %s\n", path)
		return -fuse.EIO
	}
	n, errc := fs.lookup(path)
	if errc != 0 {
		return errc
	}
	if n.isFolder {
		return -fuse.EISDIR
	}
	if size < 0 {
		return -fuse.EINVAL
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	n.size = size
	n.isDirty = true
	n.mtime = time.Now()
	if fs.staging == nil {
		return -fuse.EIO
	}
	if n.localPath == "" {
		newFid := "local_" + n.name + "_" + fmt.Sprint(time.Now().UnixNano())
		localPath, err := fs.staging.Create(newFid)
		if err != nil {
			return -fuse.EIO
		}
		n.localPath = localPath
	}
	if err := fs.staging.Truncate(n.localPath, size); err != nil {
		return -fuse.EIO
	}
	if err := fs.maybeSavePendingNodeLocked(path, n, true); err != nil {
		driver.Log.Warnf("Warning: failed to save pending node after Truncate for %s: %v\n", path, err)
	}
	return 0
}

// Flush 刷新文件 - 不再触发上传，避免写入未完成时就开始上传
func (fs *QryptFS) Flush(path string, fh uint64) (errc int) {
	return 0
}

// Chmod 当前不透传权限，仅接受请求避免 ENOSYS 触发用户态失败。
func (fs *QryptFS) Chmod(path string, mode uint32) (errc int) {
	_, errc = fs.lookup(path)
	if errc != 0 {
		return errc
	}
	return 0
}

// Chown 当前不透传属主变更，仅接受请求。
func (fs *QryptFS) Chown(path string, uid uint32, gid uint32) (errc int) {
	_, errc = fs.lookup(path)
	if errc != 0 {
		return errc
	}
	return 0
}

// Utimens 更新访问/修改时间（本地节点层面）。
func (fs *QryptFS) Utimens(path string, tmsp []fuse.Timespec) (errc int) {
	n, errc := fs.lookup(path)
	if errc != 0 {
		return errc
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if len(tmsp) > 1 {
		n.mtime = tmsp[1].Time()
	} else {
		n.mtime = time.Now()
	}
	return 0
}

// Setxattr 忽略扩展属性写入，返回成功以兼容 Finder/cp 行为。
func (fs *QryptFS) Setxattr(path string, name string, value []byte, flags int) (errc int) {
	_, errc = fs.lookup(path)
	if errc != 0 {
		return errc
	}
	return 0
}

// Getxattr 不提供扩展属性，返回 ENOATTR。
func (fs *QryptFS) Getxattr(path string, name string) (int, []byte) {
	_, errc := fs.lookup(path)
	if errc != 0 {
		return errc, nil
	}
	return -fuse.ENOATTR, nil
}

// Removexattr 不提供扩展属性，返回 ENOATTR。
func (fs *QryptFS) Removexattr(path string, name string) (errc int) {
	_, errc = fs.lookup(path)
	if errc != 0 {
		return errc
	}
	return -fuse.ENOATTR
}

// Listxattr 无扩展属性。
func (fs *QryptFS) Listxattr(path string, fill func(name string) bool) (errc int) {
	_, errc = fs.lookup(path)
	if errc != 0 {
		return errc
	}
	return 0
}

// Release 文件句柄关闭时触发最终上传。
// 设计：Write 只写 staging 不触发 sync，Release 是唯一的 sync 触发点。
// 一个文件只上传一次，避免 mtime 竞态导致的重复文件。
func (fs *QryptFS) Release(path string, fh uint64) (errc int) {
	defer func() {
		if r := recover(); r != nil {
			driver.Log.Errorf("PANIC in Release(%s): %v\n%s\n", path, r, debug.Stack())
			errc = -fuse.EIO
		}
	}()
	if fs.isShuttingDown() {
		driver.Log.Warnf("[SHUTDOWN] Rejecting Release: %s\n", path)
		// 不返回 error —— Release 的返回值在 FUSE 规范中无意义，且返回错误
		// 可能导致 OS 层无限重试
		return 0
	}
	node, errc := fs.lookup(path)
	if errc != 0 {
		return errc
	}

	node.mu.RLock()
	dirty := node.isDirty
	node.mu.RUnlock()

	if dirty {
		fs.enqueueSyncDelay(node, 200*time.Millisecond)
	}
	return 0
}
