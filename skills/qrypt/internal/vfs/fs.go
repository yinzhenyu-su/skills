package vfs

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/driver"
)

const (
	// FetchBatchBlocks 定义了一次批量下载的块数 (128 * 64KB = 8MB)
	FetchBatchBlocks     = 128
	maxAutoRetryAttempts = 5
)

var errNonRetryableSync = errors.New("non-retryable sync error")

type node struct {
	fid            string
	name           string // 明文名称
	size           int64  // 原始明文大小
	encSize        int64  // 网盘上的加密大小
	isFolder       bool
	mtime          time.Time // 修改时间
	fileNonce      [24]byte
	hasNonce       bool
	isDirty        bool  // 是否有未同步的修改
	lastReadBlock  int64 // 上次读取的块索引
	readSeqCount   int   // 连续顺序读取的块数
	mu             sync.RWMutex
}

// QryptFS 实现了 fuse.FileSystem 接口
type QryptFS struct {
	fuse.FileSystemBase
	driver     *driver.QuarkDriver
	cache      *cache.CacheManager
	cipher     *crypt.RcloneCipher
	rootFid    string
	nodes      sync.Map    // path -> *node
	fetching   sync.Map    // batchKey -> chan struct{} (用于合并请求)
	memCache   sync.Map    // fid_idx -> []byte (内存二级缓存)
	uploadChan chan string // 异步上传队列
	syncing    sync.Map    // path -> struct{} (防止并发同步同一文件)
	retryState sync.Map    // path -> int (自动重试次数)
}

// NewQryptFS 创建新的文件系统实例
func NewQryptFS(d *driver.QuarkDriver, c *cache.CacheManager, rootFid string, cipher *crypt.RcloneCipher) *QryptFS {
	fs := &QryptFS{
		driver:     d,
		cache:      c,
		rootFid:    rootFid,
		cipher:     cipher,
		uploadChan: make(chan string, 1000), // 允许排队 1000 个文件
	}
	fs.nodes.Store("/", &node{fid: rootFid, isFolder: true, mtime: time.Now(), lastReadBlock: -1})

	// 启动后台上传工作协程 (限制并发为 3)
	for i := 0; i < 3; i++ {
		go fs.uploadWorker()
	}

	// 恢复上次未完成的任务
	fs.recoverDirtyFiles()

	return fs
}

func (fs *QryptFS) recoverDirtyFiles() {
	if fs.cache == nil {
		return
	}
	pending, err := fs.cache.GetPendingNodes()
	if err != nil {
		fmt.Printf("Failed to recover dirty files: %v\n", err)
		return
	}

	for _, p := range pending {
		if p.Path == "" || !strings.HasPrefix(p.Path, "/") || p.Fid == "" || p.Name == "" || p.IsFolder {
			fmt.Printf("Drop invalid pending entry: path=%q fid=%q\n", p.Path, p.Fid)
			_ = fs.cache.RemovePendingNode(p.Path)
			if p.Fid != "" {
				_ = fs.cache.RemovePendingNodesByFid(p.Fid)
				_ = fs.cache.RemoveChunksByFid(p.Fid)
			}
			continue
		}

		n := &node{
			fid:      p.Fid,
			name:     p.Name,
			size:     p.Size,
			isFolder: p.IsFolder,
			mtime:    time.Now(),
			isDirty:  true,
		}
		if len(p.Nonce) == 24 {
			copy(n.fileNonce[:], p.Nonce)
			n.hasNonce = true
		}
		fs.nodes.Store(p.Path, n)
		fs.uploadChan <- p.Path
		fmt.Printf("Recovered pending upload: %s\n", p.Path)
	}
}

func (fs *QryptFS) uploadWorker() {
	for path := range fs.uploadChan {
		func(p string) {
			// 防止并发同步同一个路径
			if _, loaded := fs.syncing.LoadOrStore(p, struct{}{}); loaded {
				return
			}
			defer fs.syncing.Delete(p)

			v, ok := fs.nodes.Load(p)
			if !ok {
				return
			}
			n := v.(*node)
			if !n.isDirty {
				return
			}

			err := fs.syncFile(p, n)
			if err != nil {
				if strings.Contains(err.Error(), errNonRetryableSync.Error()) {
					fmt.Printf("Background Sync Non-Retryable for %s: %v\n", p, err)
					fs.retryState.Delete(p)
					fs.cleanupLocalUploadState(p, n.fid, false)
					return
				}

				attempt := 0
				if v, ok := fs.retryState.Load(p); ok {
					attempt = v.(int)
				}
				attempt++
				fs.retryState.Store(p, attempt)

				if attempt >= maxAutoRetryAttempts {
					fmt.Printf("Background Sync Error for %s reached max retries (%d): %v. Keep pending for manual retry.\n", p, attempt, err)
					return
				}

				delay := time.Duration(attempt*15) * time.Second
				fmt.Printf("Background Sync Error for %s: %v. Retrying in %s (attempt %d/%d)...\n", p, err, delay, attempt, maxAutoRetryAttempts)
				go func(pInner string, d time.Duration) {
					time.Sleep(d)
					fs.uploadChan <- pInner
				}(p, delay)
			} else {
				fs.retryState.Delete(p)
				fmt.Printf("Successfully synced %s to Quark Drive\n", p)
			}
		}(path)
	}
}

func (fs *QryptFS) cleanupLocalUploadState(path, fid string, isDir bool) {
	fs.nodes.Delete(path)
	if fs.cache != nil {
		if isDir {
			_ = fs.cache.RemovePendingNodesByPrefix(path)
		} else {
			_ = fs.cache.RemovePendingNode(path)
		}
		if fid != "" {
			_ = fs.cache.RemovePendingNodesByFid(fid)
			_ = fs.cache.RemoveChunksByFid(fid)
		}
	}
	if fid != "" {
		prefix := fid + "_"
		fs.memCache.Range(func(k, _ interface{}) bool {
			if ks, ok := k.(string); ok && strings.HasPrefix(ks, prefix) {
				fs.memCache.Delete(ks)
			}
			return true
		})
	}

	if isDir {
		prefix := path
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		fs.nodes.Range(func(k, v interface{}) bool {
			ks, ok := k.(string)
			if !ok {
				return true
			}
			if strings.HasPrefix(ks, prefix) {
				if n, ok := v.(*node); ok {
					if fs.cache != nil {
						_ = fs.cache.RemovePendingNode(ks)
						if n.fid != "" {
							_ = fs.cache.RemovePendingNodesByFid(n.fid)
							_ = fs.cache.RemoveChunksByFid(n.fid)
						}
					}
					if n.fid != "" {
						memPrefix := n.fid + "_"
						fs.memCache.Range(func(mk, _ interface{}) bool {
							if mks, ok := mk.(string); ok && strings.HasPrefix(mks, memPrefix) {
								fs.memCache.Delete(mks)
							}
							return true
						})
					}
				}
				fs.nodes.Delete(ks)
				fs.retryState.Delete(ks)
			}
			return true
		})
	}
}

func (fs *QryptFS) lookup(path string) (*node, int) {
	if v, ok := fs.nodes.Load(path); ok {
		return v.(*node), 0
	}

	if path == "/" {
		return &node{fid: fs.rootFid, isFolder: true, mtime: time.Now()}, 0
	}

	// 逐级解析
	parts := strings.Split(strings.Trim(path, "/"), "/")
	currentPath := ""
	currentFid := fs.rootFid

	for _, part := range parts {
		parentPath := currentPath
		if parentPath == "" {
			parentPath = "/"
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
			if decName == part {
				decSize, _ := fs.cipher.DecryptedSize(f.Size)
				modTime := f.ModTime()
				n := &node{
					fid:      f.Fid,
					name:     decName,
					size:     decSize,
					encSize:  f.Size,
					isFolder: f.IsDir(),
					mtime:    modTime,
				}
				fs.nodes.Store(currentPath, n)
				currentFid = f.Fid
				found = true
				break
			}
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

	n, errc := fs.lookup(path)
	if errc != 0 {
		return errc
	}

	uid, gid, _ := fuse.Getcontext()
	stat.Uid = uid
	stat.Gid = gid

	if n.isFolder {
		stat.Mode = fuse.S_IFDIR | 0777
	} else {
		stat.Mode = fuse.S_IFREG | 0666
		stat.Size = n.size
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

	n, errc := fs.lookup(path)
	if errc != 0 {
		return errc
	}

	files, err := fs.driver.ListFiles(n.fid)
	if err != nil {
		return -fuse.EIO
	}

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
				continue
			}
			if fs.cache != nil {
				_ = fs.cache.SaveCachedName(f.Fid, f.FileName, decName)
			}
		}

		stat := &fuse.Stat_t{}
		uid, gid, _ := fuse.Getcontext()
		stat.Uid = uid
		stat.Gid = gid

		decSize, _ := fs.cipher.DecryptedSize(f.Size)
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

		// 顺便存入缓存
		childPath := path
		if !strings.HasSuffix(childPath, "/") {
			childPath += "/"
		}
		childPath += decName
		fs.nodes.Store(childPath, &node{
			fid:      f.Fid,
			name:     decName,
			size:     decSize,
			encSize:  f.Size,
			isFolder: f.IsDir(),
			mtime:    modTime,
		})

		fill(decName, stat, 0)
	}

	return 0
}

// Mkdir 创建文件夹
func (fs *QryptFS) Mkdir(path string, mode uint32) (errc int) {
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

	// 加入缓存
	fs.nodes.Store(path, &node{
		fid:      fid,
		name:     name,
		isFolder: true,
		mtime:    time.Now(),
	})

	return 0
}

// Unlink 删除文件
func (fs *QryptFS) Unlink(path string) (errc int) {
	n, errc := fs.lookup(path)
	if errc != 0 {
		return errc
	}

	if n.isFolder {
		return -fuse.EISDIR
	}

	err := fs.driver.Delete([]string{n.fid})
	if err != nil {
		fmt.Printf("Unlink failed for %s (fid=%s): %v\n", path, n.fid, err)
		return -fuse.EIO
	}

	fs.cleanupLocalUploadState(path, n.fid, false)
	return 0
}

// Rmdir 删除文件夹
func (fs *QryptFS) Rmdir(path string) (errc int) {
	n, errc := fs.lookup(path)
	if errc != 0 {
		return errc
	}

	if !n.isFolder {
		return -fuse.ENOTDIR
	}

	err := fs.driver.Delete([]string{n.fid})
	if err != nil {
		fmt.Printf("Rmdir failed for %s (fid=%s): %v\n", path, n.fid, err)
		return -fuse.EIO
	}

	fs.cleanupLocalUploadState(path, n.fid, true)
	return 0
}

// Rename 重命名或移动文件
func (fs *QryptFS) Rename(oldPath string, newPath string) (errc int) {
	oldNode, errc := fs.lookup(oldPath)
	if errc != 0 {
		return errc
	}

	oldParent := filepath.Dir(oldPath)
	newParent := filepath.Dir(newPath)
	newName := filepath.Base(newPath)

	if oldParent != newParent {
		// 跨目录移动
		newParentNode, errc := fs.lookup(newParent)
		if errc != 0 {
			return errc
		}
		err := fs.driver.Move([]string{oldNode.fid}, newParentNode.fid)
		if err != nil {
			return -fuse.EIO
		}
	}

	// 始终尝试重命名（夸克 API 允许移动和重命名分开或合并，这里简单化处理）
	encName := fs.cipher.EncryptSegment(newName)
	err := fs.driver.Rename(oldNode.fid, encName)
	if err != nil {
		return -fuse.EIO
	}

	// 更新缓存
	fs.nodes.Delete(oldPath)
	oldNode.name = newName
	fs.nodes.Store(newPath, oldNode)

	if oldNode.isFolder {
		// 递归更新所有子节点的路径
		oldPrefix := oldPath
		if !strings.HasSuffix(oldPrefix, "/") {
			oldPrefix += "/"
		}
		newPrefix := newPath
		if !strings.HasSuffix(newPrefix, "/") {
			newPrefix += "/"
		}

		fs.nodes.Range(func(key, value interface{}) bool {
			path, ok := key.(string)
			if !ok {
				return true
			}
			if strings.HasPrefix(path, oldPrefix) {
				childNode := value.(*node)
				relative := strings.TrimPrefix(path, oldPrefix)
				newChildPath := newPrefix + relative
				
				// 标记为删除旧路径，存入新路径
				fs.nodes.Delete(path)
				fs.nodes.Store(newChildPath, childNode)
			}
			return true
		})
	}

	return 0
}

// Open 打开文件
func (fs *QryptFS) Open(path string, flags int) (errc int, fh uint64) {
	_, errc = fs.lookup(path)
	return errc, 0
}

// Access 访问检查（在 defer_permissions 下仍提供显式允许，避免 ENOSYS 被解释为权限错误）
func (fs *QryptFS) Access(path string, mask uint32) (errc int) {
	if strings.Contains(path, "/.DS_Store") || strings.Contains(path, "/._") {
		return -fuse.ENOENT
	}
	_, errc = fs.lookup(path)
	if errc != 0 {
		return errc
	}
	return 0
}

// Create 创建新文件
func (fs *QryptFS) Create(path string, flags int, mode uint32) (errc int, fh uint64) {
	if strings.Contains(path, "/.DS_Store") || strings.Contains(path, "/._") {
		return -fuse.ENOENT, 0
	}
	// 初始化一个临时本地节点
	name := filepath.Base(path)
	n := &node{
		fid:      "local_" + name + "_" + fmt.Sprint(time.Now().UnixNano()),
		name:     name,
		size:     0,
		isFolder: false,
		mtime:    time.Now(),
		isDirty:  true,
	}

	nonce, err := fs.cipher.GenerateRandomNonce()
	if err == nil {
		n.fileNonce = nonce
		n.hasNonce = true
	}

	fs.nodes.Store(path, n)
	if fs.cache != nil {
		_ = fs.cache.SavePendingNode(path, n.fid, n.name, n.size, n.isFolder, n.fileNonce[:])
	}
	return 0, 0
}

// Mknod 部分 macOS 写入路径会触发 Mknod，转发到 Create 统一处理。
func (fs *QryptFS) Mknod(path string, mode uint32, dev uint64) (errc int) {
	err, _ := fs.Create(path, 0, mode)
	return err
}

// Write 写入文件内容
func (fs *QryptFS) Write(path string, buff []byte, ofst int64, fh uint64) (n int) {
	if strings.Contains(path, "/.DS_Store") || strings.Contains(path, "/._") {
		return 0
	}
	node, errc := fs.lookup(path)
	if errc != 0 {
		return 0
	}

	node.mu.Lock()
	defer node.mu.Unlock()

	// 1. 更新节点信息
	if ofst+int64(len(buff)) > node.size {
		node.size = ofst + int64(len(buff))
	}
	node.isDirty = true
	node.mtime = time.Now()

	// 2. 写入分块缓存
	startChunk := ofst / crypt.BlockDataSize
	endChunk := (ofst + int64(len(buff)) - 1) / crypt.BlockDataSize

	totalWritten := 0
	for i := startChunk; i <= endChunk; i++ {
		// 获取原有块内容（如果需要部分写入）
		var chunkData []byte
		if ofst%crypt.BlockDataSize != 0 || int64(len(buff)) < crypt.BlockDataSize {
			// 需要读取原始块
			orig, err := fs.getDecryptedChunk(node, uint64(i))
			if err == nil {
				chunkData = orig
			}
		}

		if chunkData == nil {
			chunkData = make([]byte, 0, crypt.BlockDataSize)
		}

		// 填充或截断 chunkData 到当前写入位置
		relOffset := int64(0)
		if i == startChunk {
			relOffset = ofst % crypt.BlockDataSize
		}

		// 确保 chunkData 长度足够
		if int64(len(chunkData)) < relOffset {
			newChunk := make([]byte, relOffset)
			copy(newChunk, chunkData)
			chunkData = newChunk
		}

		// 准备写入的数据
		writeStart := int64(totalWritten)
		writeEnd := writeStart + (crypt.BlockDataSize - relOffset)
		if writeEnd > int64(len(buff)) {
			writeEnd = int64(len(buff))
		}

		payload := buff[writeStart:writeEnd]

		// 合并到 chunkData
		if int64(len(chunkData)) < relOffset+int64(len(payload)) {
			newChunk := make([]byte, relOffset+int64(len(payload)))
			copy(newChunk, chunkData)
			chunkData = newChunk
		}
		copy(chunkData[relOffset:], payload)

		// 存入缓存并标记为脏
		if fs.cache != nil {
			_ = fs.cache.PutChunk(node.fid, int64(i), chunkData, true)
		}
		// 同时存入内存缓存
		mKey := fmt.Sprintf("%s_%d", node.fid, i)
		fs.memCache.Store(mKey, chunkData)

		totalWritten += len(payload)
	}

	// 持久化节点元数据
	if fs.cache != nil {
		_ = fs.cache.SavePendingNode(path, node.fid, node.name, node.size, node.isFolder, node.fileNonce[:])
	}

	return totalWritten
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
	if fs.cache != nil {
		_ = fs.cache.SavePendingNode(path, n.fid, n.name, n.size, n.isFolder, n.fileNonce[:])
	}
	return 0
}

// Flush 刷新文件
func (fs *QryptFS) Flush(path string, fh uint64) (errc int) {
	node, errc := fs.lookup(path)
	if errc != 0 {
		return errc
	}

	if node.isDirty {
		// 放入异步队列，不再阻塞当前线程
		select {
		case fs.uploadChan <- path:
			fmt.Printf("File %s queued for upload\n", path)
		default:
			fmt.Printf("Upload queue full, blocking for %s\n", path)
			fs.uploadChan <- path
		}
	}

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

// Release 文件句柄关闭时触发，沿用 Flush 行为以覆盖更多编辑器写入路径
func (fs *QryptFS) Release(path string, fh uint64) (errc int) {
	return fs.Flush(path, fh)
}

func (fs *QryptFS) syncFile(path string, n *node) error {
	n.mu.Lock()
	if !n.isDirty {
		n.mu.Unlock()
		return nil
	}
	snapshotSize := n.size
	snapshotName := n.name
	n.isDirty = false
	n.mu.Unlock()

	fmt.Printf("Syncing file (Parallel): %s (size %d)\n", snapshotName, snapshotSize)

	parentPath := filepath.Dir(path)
	parentNode, parentErr := fs.lookup(parentPath)
	if parentErr != 0 {
		return fmt.Errorf("%w: parent path not found for %s", errNonRetryableSync, path)
	}

	newNonce, _ := fs.cipher.GenerateRandomNonce()
	encName := fs.cipher.EncryptSegment(snapshotName)
	pre, err := fs.driver.UploadPre(encName, parentNode.fid, fs.cipher.EncryptedSize(snapshotSize))
	if err != nil {
		return err
	}

	// 检查秒传
	if pre.Data.Finish {
		fmt.Printf("Rapid Upload (秒传) triggered for %s\n", snapshotName)
		n.mu.Lock()
		n.fid = pre.Data.Fid
		if fs.cache != nil {
			_ = fs.cache.RemovePendingNode(path)
		}
		n.mu.Unlock()
		return nil
	}

	// 计算总块数 (rclone blocks)
	totalRcloneBlocks := (snapshotSize + crypt.BlockDataSize - 1) / crypt.BlockDataSize
	if snapshotSize == 0 {
		totalRcloneBlocks = 0
	}

	// 策略：每 64 个 rclone block (约 4MB) 作为一个 Quark Part 上传
	blocksPerPart := 64
	numParts := (int(totalRcloneBlocks) + blocksPerPart - 1) / blocksPerPart
	if numParts == 0 {
		numParts = 1
	}

	// OSS 要求除最后一个 part 外都满足最小大小限制，因此将 rclone header 并入第一个数据 part。
	etags := make([]string, numParts)
	var wg sync.WaitGroup
	errChan := make(chan error, numParts)

	// 限制单个文件的并发上传数为 4
	semaphore := make(chan struct{}, 4)

	// 并发上传分块；第一个 OSS part 携带 rclone header，避免产生过小的非末尾分片。
	for p := 0; p < numParts; p++ {
		wg.Add(1)
		go func(partIdx int) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			var partBuffer bytes.Buffer
			if partIdx == 0 {
				partBuffer.Write([]byte(crypt.FileMagic))
				partBuffer.Write(newNonce[:])
			}
			startBlock := int64(partIdx * blocksPerPart)
			endBlock := startBlock + int64(blocksPerPart)
			if endBlock > totalRcloneBlocks {
				endBlock = totalRcloneBlocks
			}

			// 读取并加密该 Part 内的所有 blocks
			for i := startBlock; i < endBlock; i++ {
				plaintext, err := fs.getDecryptedChunk(n, uint64(i))
				if err != nil {
					errChan <- err
					return
				}
				ciphertext, err := fs.cipher.EncryptBlock(plaintext, uint64(i), newNonce)
				if err != nil {
					errChan <- err
					return
				}
				partBuffer.Write(ciphertext)
			}

			partNum := partIdx + 1
			etag, err := fs.driver.UploadPart(pre, partNum, partBuffer.Bytes())
			if err != nil {
				errChan <- err
				return
			}
			etags[partIdx] = etag
		}(p)
	}

	wg.Wait()
	close(errChan)

	if len(errChan) > 0 {
		return <-errChan
	}

	md5Hex, sha1Hex, err := fs.computeEncryptedHashes(n, newNonce, snapshotSize)
	if err != nil {
		return err
	}

	finish, fid, err := fs.driver.UpdateHash(strings.ToUpper(md5Hex), strings.ToUpper(sha1Hex), pre.Data.TaskId)
	if err != nil {
		fmt.Printf("[DEBUG] UpdateHash error: %v\n", err)
		return err
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	if finish {
		fmt.Printf("[DEBUG] UpdateHash finish=true for %s, skip commit fallback\n", path)
		if fid != "" {
			n.fid = fid
		} else {
			n.fid = pre.Data.Fid
		}
		n.fileNonce = newNonce
		n.hasNonce = true
		n.encSize = fs.cipher.EncryptedSize(snapshotSize)
		if fs.cache != nil {
			_ = fs.cache.RemovePendingNode(path)
		}
		return nil
	}
	fmt.Printf("[DEBUG] UpdateHash finish=false for %s, fallback to commit/finish\n", path)

	// 4. 提交
	fmt.Printf("[DEBUG] UploadCommit: etags=%v\n", etags)
	err = fs.driver.UploadCommit(pre, etags)
	if err != nil {
		fmt.Printf("[DEBUG] UploadCommit error: %v\n", err)
		return err
	}
	fmt.Printf("[DEBUG] UploadFinish: objKey=%s, taskId=%s\n", pre.Data.ObjKey, pre.Data.TaskId)
	err = fs.driver.UploadFinish(pre)
	if err != nil {
		fmt.Printf("[DEBUG] UploadFinish error: %v\n", err)
		return err
	}

	// 5. 更新状态
	n.fid = pre.Data.Fid
	n.fileNonce = newNonce
	n.hasNonce = true
	n.encSize = fs.cipher.EncryptedSize(snapshotSize)

	// 6. 从持久化队列移除
	if fs.cache != nil {
		_ = fs.cache.RemovePendingNode(path)
	}

	return nil
}

func (fs *QryptFS) computeEncryptedHashes(n *node, newNonce [24]byte, snapshotSize int64) (string, string, error) {
	md5h := md5.New()
	sha1h := sha1.New()

	header := append([]byte(crypt.FileMagic), newNonce[:]...)
	_, _ = md5h.Write(header)
	_, _ = sha1h.Write(header)

	totalRcloneBlocks := (snapshotSize + crypt.BlockDataSize - 1) / crypt.BlockDataSize
	for i := int64(0); i < totalRcloneBlocks; i++ {
		plaintext, err := fs.getDecryptedChunk(n, uint64(i))
		if err != nil {
			return "", "", err
		}
		ciphertext, err := fs.cipher.EncryptBlock(plaintext, uint64(i), newNonce)
		if err != nil {
			return "", "", err
		}
		_, _ = md5h.Write(ciphertext)
		_, _ = sha1h.Write(ciphertext)
	}

	return hex.EncodeToString(md5h.Sum(nil)), hex.EncodeToString(sha1h.Sum(nil)), nil
}

// Read 读取文件内容
func (fs *QryptFS) Read(path string, buff []byte, ofst int64, fh uint64) (n int) {
	node, errc := fs.lookup(path)
	if errc != 0 {
		return 0
	}

	if ofst >= node.size {
		return 0
	}

	// 计算涉及的分块
	startChunk := ofst / crypt.BlockDataSize
	endChunk := (ofst + int64(len(buff)) - 1) / crypt.BlockDataSize

	// 检测顺序读取：
	// 如果本次读取开始于上次读取结束的下一块，则视为顺序读取
	if startChunk == node.lastReadBlock+1 {
		node.readSeqCount += int(endChunk - startChunk + 1)
	} else {
		node.readSeqCount = int(endChunk - startChunk + 1)
	}
	node.lastReadBlock = endChunk

	// 启动后台预取：
	// 只有在满足顺序读取条件（比如连续读了 2 个块以上）时才触发预取
	if node.readSeqCount >= 2 {
		nextBatchIdx := (uint64(endChunk) / FetchBatchBlocks) + 1
		go fs.prefetchBatch(node, nextBatchIdx)
	}

	totalRead := 0
	for i := startChunk; i <= endChunk; i++ {
		// 获取解密后的分块数据
		data, err := fs.getDecryptedChunk(node, uint64(i))
		if err != nil {
			break
		}

		chunkStart := int64(0)
		if i == startChunk {
			chunkStart = ofst % crypt.BlockDataSize
		}

		chunkEnd := int64(len(data))
		remaining := int64(len(buff)) - int64(totalRead)
		if chunkEnd-chunkStart > remaining {
			chunkEnd = chunkStart + remaining
		}

		if chunkStart >= int64(len(data)) {
			continue
		}

		copied := copy(buff[totalRead:], data[chunkStart:chunkEnd])
		totalRead += copied

		if int64(totalRead) >= int64(len(buff)) {
			break
		}
	}

	return totalRead
}

// prefetchBatch 异步下载批次 (支持根据顺序读取长度进行动态调整)
func (fs *QryptFS) prefetchBatch(n *node, batchIdx uint64) {
	// 1. 基础预取：预取当前批次
	if int64(batchIdx)*FetchBatchBlocks*crypt.BlockDataSize >= n.size {
		return
	}
	// 尝试获取该批次的第一个块，会自动触发整个批次的下载
	_, _ = fs.getDecryptedChunk(n, batchIdx*FetchBatchBlocks)

	// 2. 动态调整：如果顺序读取长度超过一个批次 (8MB)，则多预取一个批次
	if n.readSeqCount > FetchBatchBlocks {
		extraBatchIdx := batchIdx + 1
		if int64(extraBatchIdx)*FetchBatchBlocks*crypt.BlockDataSize < n.size {
			_, _ = fs.getDecryptedChunk(n, extraBatchIdx*FetchBatchBlocks)
		}
	}
}

// getDecryptedChunk 核心逻辑：获取、解密并缓存分块 (支持内存/磁盘双层缓存)
func (fs *QryptFS) getDecryptedChunk(n *node, chunkIndex uint64) ([]byte, error) {
	memKey := fmt.Sprintf("%s_%d", n.fid, chunkIndex)

	// 1. 优先查内存缓存
	if val, ok := fs.memCache.Load(memKey); ok {
		return val.([]byte), nil
	}

	// 2. 查磁盘缓存
	if fs.cache != nil {
		cachedData, err := fs.cache.GetChunk(n.fid, int64(chunkIndex))
		if err == nil && cachedData != nil {
			fs.memCache.Store(memKey, cachedData) // 回填内存
			return cachedData, nil
		}
	}

	// 3. 触发批量下载
	batchIdx := chunkIndex / FetchBatchBlocks
	batchKey := fmt.Sprintf("%s_b%d", n.fid, batchIdx)

	// 请求合并 (Request Coalescing)
	ch := make(chan struct{})
	actual, loaded := fs.fetching.LoadOrStore(batchKey, ch)
	if loaded {
		<-actual.(chan struct{})
		// 下载完成后重新调用自己，此时应能命中缓存
		return fs.getDecryptedChunk(n, chunkIndex)
	}

	defer func() {
		close(ch)
		fs.fetching.Delete(batchKey)
	}()

	// 执行批量下载
	err := fs.downloadBatch(n, batchIdx)
	if err != nil {
		return nil, err
	}

	return fs.getDecryptedChunk(n, chunkIndex)
}

func (fs *QryptFS) downloadBatch(n *node, batchIdx uint64) error {
	url, err := fs.driver.GetDownloadURL(n.fid)
	if err != nil {
		return err
	}

	if !n.hasNonce {
		header, err := fs.getFileHeader(url)
		if err != nil {
			return err
		}
		if len(header) < crypt.FileHeaderSize {
			return fmt.Errorf("file too short for rclone header")
		}
		copy(n.fileNonce[:], header[crypt.FileMagicSize:crypt.FileHeaderSize])
		n.hasNonce = true
	}

	startBlock := batchIdx * FetchBatchBlocks
	endBlock := startBlock + FetchBatchBlocks - 1

	pStart := int64(crypt.FileHeaderSize) + int64(startBlock)*int64(crypt.BlockSize)
	if pStart >= n.encSize {
		return io.EOF
	}

	pEnd := int64(crypt.FileHeaderSize) + int64(endBlock+1)*int64(crypt.BlockSize) - 1
	if pEnd >= n.encSize {
		pEnd = n.encSize - 1
	}

	fmt.Printf("Batch Fetch: '%s' Blocks %d-%d\n", n.name, startBlock, endBlock)

	rc, err := fs.driver.DownloadChunk(url, pStart, pEnd)
	if err != nil {
		return err
	}
	defer rc.Close()

	for i := startBlock; i <= endBlock; i++ {
		encBuf := make([]byte, crypt.BlockSize)
		nRead, err := io.ReadFull(rc, encBuf)
		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			return err
		}
		if nRead == 0 {
			break
		}

		plaintext, err := fs.cipher.DecryptBlock(encBuf[:nRead], uint64(i), n.fileNonce)
		if err != nil {
			return err
		}

		// 存入内存和磁盘双级缓存
		mKey := fmt.Sprintf("%s_%d", n.fid, i)
		fs.memCache.Store(mKey, plaintext)
		if fs.cache != nil {
			_ = fs.cache.PutChunk(n.fid, int64(i), plaintext, false)
		}

		if err == io.ErrUnexpectedEOF || err == io.EOF {
			break
		}
	}

	return nil
}

func (fs *QryptFS) getFileHeader(url string) ([]byte, error) {
	rc, err := fs.driver.DownloadChunk(url, 0, int64(crypt.FileHeaderSize)-1)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}
