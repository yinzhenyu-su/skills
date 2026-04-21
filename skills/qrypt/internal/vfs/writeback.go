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

// Create 创建新文件
func (fs *QryptFS) Create(path string, flags int, mode uint32) (errc int, fh uint64) {
	driver.Log.Printf("[FUSE] Create: path=%s, flags=%d, mode=%o\n", path, flags, mode)
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
	n.mu.Lock()
	if err := fs.maybeSavePendingNodeLocked(path, n, true); err != nil {
		driver.Log.Printf("Warning: failed to save pending node after Create for %s: %v\n", path, err)
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
	driver.Log.Printf("[FUSE] Write: path=%s, len=%d, offset=%d, fh=%d\n", path, len(buff), ofst, fh)
	if strings.Contains(path, "/.DS_Store") || strings.Contains(path, "/._") {
		return 0
	}
	node, errc := fs.lookup(path)
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
			driver.Log.Printf("Warning: failed to create staging file for Write on %s: %v\n", path, err)
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
		driver.Log.Printf("Warning: failed to save pending node after Write for %s: %v\n", path, err)
	}
	node.mu.Unlock()

	// Trigger sync from Write (not just Release) to handle FUSE early-Release on macOS.
	// The kernel may call Release before Write completes, so we sync from Write directly.
	fs.enqueueSync(node)
	return written
}

// Truncate 调整文件大小（用于 cp/touch 等写入前截断流程）
func (fs *QryptFS) Truncate(path string, size int64, fh uint64) (errc int) {
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
		driver.Log.Printf("Warning: failed to save pending node after Truncate for %s: %v\n", path, err)
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

// Release 文件句柄关闭时触发。主要同步由 Write 触发，这里作为兜底。
func (fs *QryptFS) Release(path string, fh uint64) (errc int) {
	node, errc := fs.lookup(path)
	if errc != 0 {
		return errc
	}

	node.mu.RLock()
	localPath := node.localPath
	isDirty := node.isDirty
	node.mu.RUnlock()

	if !isDirty {
		// Never written (e.g., `touch`). Sync to create a valid 0-byte file on server.
		fs.enqueueSync(node)
		return 0
	}

	// isDirty=true: data was written. Check if staging file has content before syncing.
	// On macOS, FUSE may call Release before Write completes, leaving staging empty.
	// In that case, skip — Write() already triggers sync directly.
	if localPath != "" {
		if size, err := fs.staging.FileSize(localPath); err == nil && size > 0 {
			fs.enqueueSync(node)
		} else {
			driver.Log.Printf("Release: skipping sync for %s (staging empty but isDirty=true, Write will sync)\n", path)
		}
	}
	return 0
}
