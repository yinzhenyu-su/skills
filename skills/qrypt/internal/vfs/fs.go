package vfs

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/driver"
	"github.com/yinzhenyu/skills/qrypt/internal/staging"
	uploadpkg "github.com/yinzhenyu/skills/qrypt/internal/upload"
)

const (
	// FetchBatchBlocks 定义了一次批量下载的块数 (128 * 64KB = 8MB)
	FetchBatchBlocks        = 128
	maxAutoRetryAttempts    = 5
	pendingNodeSaveInterval = 250 * time.Millisecond
	pendingNodeSaveSizeStep = 1 * 1024 * 1024
)

var errNonRetryableSync = errors.New("non-retryable sync error")

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

type node struct {
	fid             string
	parentFid       string // 父目录 FID
	name            string // 明文名称
	size            int64  // 原始明文大小
	encSize         int64  // 网盘上的加密大小
	localPath       string // 本地 staging 文件
	isFolder        bool
	mtime           time.Time // 修改时间
	fileNonce       [24]byte
	hasNonce        bool
	isDirty         bool  // 是否有未同步的修改
	syncQueued      bool  // 是否已在同步队列中
	lastReadBlock   int64 // 上次读取的块索引
	readSeqCount    int   // 连续顺序读取的块数
	lastPendingSave time.Time
	lastPendingSize int64
	mu              sync.RWMutex
}

type syncTask struct {
	node *node
	path string
}

type syncPerformanceSnapshot struct {
	Path               string
	SnapshotSize       int64
	PartCount          int
	UploadedBytes      int64
	PreDuration        time.Duration
	UploadPartDuration time.Duration
	UpdateHashDuration time.Duration
	CommitDuration     time.Duration
	FinishDuration     time.Duration
	TotalDuration      time.Duration
}

type syncObserver interface {
	OnSyncStart(path string, snapshotSize int64)
	OnSyncFinish(snapshot syncPerformanceSnapshot, err error)
}

// QryptFS 实现了 fuse.FileSystem 接口
type QryptFS struct {
	fuse.FileSystemBase
	driver       *driver.QuarkDriver
	cache        *cache.CacheManager
	cipher       *crypt.RcloneCipher
	rootFid      string
	nodes        sync.Map // path -> *node
	fetching     sync.Map // batchKey -> chan struct{} (用于合并请求)
	memCache     sync.Map // fid_idx -> []byte (内存二级缓存)
	uploadChan   chan syncTask
	syncing      sync.Map // *node -> struct{} (防止并发同步同一节点)
	retryState   sync.Map // *node -> int (基于节点的自动重试次数)
	syncObserver syncObserver
	staging      *staging.Store
	uploader     *uploadpkg.Manager
}

// NewQryptFS 创建新的文件系统实例
func NewQryptFS(d *driver.QuarkDriver, c *cache.CacheManager, rootFid string, cipher *crypt.RcloneCipher) *QryptFS {
	var stagingStore *staging.Store
	if c != nil {
		s, err := staging.NewStore(c.StagingDir())
		if err != nil {
			driver.Log.Printf("Failed to initialize staging store: %v\n", err)
		} else {
			stagingStore = s
		}
	}
	fs := &QryptFS{
		driver:     d,
		cache:      c,
		rootFid:    rootFid,
		cipher:     cipher,
		uploadChan: make(chan syncTask, 1000), // 允许排队 1000 个文件
		staging:    stagingStore,
	}
	if stagingStore != nil {
		fs.uploader = uploadpkg.NewManager(d, cipher, stagingStore)
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
		driver.Log.Printf("Failed to recover dirty files: %v\n", err)
		return
	}

	for _, p := range pending {
		if p.Path == "" || !strings.HasPrefix(p.Path, "/") || p.Fid == "" || p.Name == "" || p.IsFolder || p.LocalPath == "" {
			driver.Log.Printf("Drop invalid pending entry: path=%q fid=%q\n", p.Path, p.Fid)
			_ = fs.cache.RemovePendingNode(p.Path)
			if p.Fid != "" {
				_ = fs.cache.RemovePendingNodesByFid(p.Fid)
				_ = fs.cache.RemoveChunksByFid(p.Fid)
			}
			continue
		}
		if fs.staging == nil || !fs.staging.Exists(p.LocalPath) {
			driver.Log.Printf("Drop pending entry with missing staging file: path=%q localPath=%q\n", p.Path, p.LocalPath)
			_ = fs.cache.RemovePendingNode(p.Path)
			continue
		}

		n := &node{
			fid:       p.Fid,
			parentFid: p.ParentFid,
			name:      p.Name,
			localPath: p.LocalPath,
			size:      p.Size,
			isFolder:  p.IsFolder,
			mtime:     time.Now(),
			isDirty:   true,
		}
		if len(p.Nonce) == 24 {
			copy(n.fileNonce[:], p.Nonce)
			n.hasNonce = true
		}
		fs.nodes.Store(p.Path, n)
		fs.uploadChan <- syncTask{node: n, path: p.Path}
		driver.Log.Printf("Recovered pending upload: %s\n", p.Path)
	}
}

func (fs *QryptFS) uploadWorker() {
	for task := range fs.uploadChan {
		func(t syncTask) {
			n := t.node
			p := t.path

			// 2.1 基于节点互斥，防止同一个文件在多路径下并发同步
			if _, loaded := fs.syncing.LoadOrStore(n, struct{}{}); loaded {
				return
			}
			defer fs.syncing.Delete(n)

			// 2.3 验证节点路径。如果路径已变，尝试寻找新路径
			actualNode, ok := fs.nodes.Load(p)
			if !ok || actualNode != n {
				// 路径已失效或指向其他节点，尝试全局搜索该节点的新路径
				found := false
				fs.nodes.Range(func(key, value interface{}) bool {
					if value == n {
						p = key.(string)
						found = true
						return false
					}
					return true
				})
				if !found {
					// 节点已从缓存移除，可能已被删除
					return
				}
			}

			if !n.isDirty {
				n.mu.Lock()
				n.syncQueued = false
				n.mu.Unlock()
				return
			}

			// 3.1 改进日志，包含 FID
			driver.Log.Printf("Background Sync Start: %s (fid=%s)\n", p, n.fid)
			fs.notifySyncStart(p, n)

			err := fs.syncFile(p, n)

			n.mu.Lock()
			n.syncQueued = false
			// 如果在同步过程中文件又变脏了（例如又有新的 Write），重新入队
			if n.isDirty {
				n.syncQueued = true
				n.mu.Unlock()
				fs.uploadChan <- syncTask{node: n, path: p}
			} else {
				n.mu.Unlock()
			}

			if err != nil {
				if strings.Contains(err.Error(), errNonRetryableSync.Error()) {
					driver.Log.Printf("Background Sync Non-Retryable for %s (fid=%s): %v\n", p, n.fid, err)
					fs.retryState.Delete(n)
					fs.cleanupLocalUploadState(p, n, false)
					return
				}

				// 2.4 基于节点的重试状态管理
				attempt := 0
				if v, ok := fs.retryState.Load(n); ok {
					attempt = v.(int)
				}
				attempt++
				fs.retryState.Store(n, attempt)

				if attempt >= maxAutoRetryAttempts {
					driver.Log.Printf("Background Sync Error for %s (fid=%s) reached max retries (%d): %v. Keep pending for manual retry.\n", p, n.fid, attempt, err)
					return
				}

				delay := time.Duration(attempt*15) * time.Second
				driver.Log.Printf("Background Sync Error for %s (fid=%s): %v. Retrying in %s (attempt %d/%d)...\n", p, n.fid, err, delay, attempt, maxAutoRetryAttempts)

				go func(nodeToRetry *node, lastPath string, d time.Duration) {
					time.Sleep(d)
					fs.uploadChan <- syncTask{node: nodeToRetry, path: lastPath}
				}(n, p, delay)
			} else {
				fs.retryState.Delete(n)
				driver.Log.Printf("Successfully synced %s (fid=%s) to Quark Drive\n", p, n.fid)
				// 同步成功后，清理父目录缓存
				n.mu.RLock()
				parentFid := n.parentFid
				n.mu.RUnlock()
				fs.driver.RemoveDirCache(parentFid)
			}
		}(task)
	}
}

func (fs *QryptFS) cleanupLocalUploadState(path string, n *node, isDir bool) {
	fs.nodes.Delete(path)
	if fs.cache != nil {
		if isDir {
			if fs.staging != nil {
				pending, err := fs.cache.GetPendingNodes()
				if err == nil {
					prefix := path
					if !strings.HasSuffix(prefix, "/") {
						prefix += "/"
					}
					for _, entry := range pending {
						if entry.Path == path || strings.HasPrefix(entry.Path, prefix) {
							_ = fs.staging.Remove(entry.LocalPath)
						}
					}
				}
			}
			_ = fs.cache.RemovePendingNodesByPrefix(path)
		} else {
			_ = fs.cache.RemovePendingNode(path)
			_ = fs.cache.RemovePendingNodesByFid(n.fid)
			_ = fs.cache.RemoveChunksByFid(n.fid)
		}
	}
	if fs.staging != nil && !isDir {
		_ = fs.staging.Remove(n.localPath)
	}
}

func (fs *QryptFS) maybeSavePendingNodeLocked(path string, n *node, force bool) {
	if fs.cache == nil {
		return
	}

	now := time.Now()
	needSave := force || n.lastPendingSave.IsZero() || now.Sub(n.lastPendingSave) >= pendingNodeSaveInterval
	sizeDelta := n.size - n.lastPendingSize
	if sizeDelta < 0 {
		sizeDelta = -sizeDelta
	}
	if sizeDelta >= pendingNodeSaveSizeStep {
		needSave = true
	}
	if !needSave {
		return
	}

	if err := fs.cache.SavePendingNode(path, n.fid, n.parentFid, n.name, n.localPath, n.size, n.isFolder, n.fileNonce[:]); err == nil {
		n.lastPendingSave = now
		n.lastPendingSize = n.size
	}
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

func (fs *QryptFS) lookup(path string) (*node, int) {
	if v, ok := fs.nodes.Load(path); ok {
		return v.(*node), 0
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
			if decName == part {
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
					fid:       f.Fid,
					parentFid: currentFid,
					name:      decName,
					size:      decSize,
					encSize:   f.Int64Size(),
					isFolder:  f.IsDir(),
					mtime:     modTime,
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

		// 顺便存入缓存
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
			fs.nodes.Store(childPath, &node{
				fid:       f.Fid,
				parentFid: n.fid,
				name:      decName,
				size:      decSize,
				isFolder:  f.IsDir(),
				mtime:     modTime,
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

	n := &node{
		fid:       fid,
		parentFid: parentNode.fid,
		name:      name,
		size:      0,
		isFolder:  true,
		mtime:     time.Now(),
	}
	fs.nodes.Store(path, n)
	// 清理父目录缓存
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
	fs.nodes.Delete(path)
	// 清理父目录缓存
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
	fs.nodes.Delete(path)
	// 清理父目录缓存
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
		// 跨目录移动
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

		// 清理原父目录和新父目录的缓存
		oldParentNode, errc := fs.lookup(oldParent)
		if errc == 0 {
			fs.driver.RemoveDirCache(oldParentNode.fid)
		}
		fs.driver.RemoveDirCache(newParentNode.fid)
	}

	// 始终尝试重命名（夸克 API 允许移动和重命名分开或合并，这里简单化处理）
	if !isLocal {
		encName := fs.cipher.EncryptSegment(newName)
		err := fs.driver.Rename(oldNode.fid, encName)
		if err != nil {
			return -fuse.EIO
		}
	}

	// 更新缓存
	fs.nodes.Delete(oldPath)
	oldNode.mu.Lock()
	oldNode.name = newName
	oldNode.mu.Unlock()
	fs.nodes.Store(newPath, oldNode)

	// 清理父目录缓存
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

	// 如果是本地节点，同步更新持久化缓存中的路径
	if isLocal && fs.cache != nil {
		oldNode.mu.RLock()
		_ = fs.cache.SavePendingNode(newPath, oldNode.fid, oldNode.parentFid, oldNode.name, oldNode.localPath, oldNode.size, oldNode.isFolder, oldNode.fileNonce[:])
		oldNode.mu.RUnlock()
		_ = fs.cache.RemovePendingNode(oldPath)
	}

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

// Create 创建新文件
func (fs *QryptFS) Create(path string, flags int, mode uint32) (errc int, fh uint64) {
	driver.Log.Printf("[FUSE] Create: path=%s, flags=%d, mode=%o\n", path, flags, mode)
	if strings.Contains(path, "/.DS_Store") || strings.Contains(path, "/._") {
		return -fuse.ENOENT, 0
	}
	// 初始化一个临时本地节点
	parentPath := filepath.Dir(path)
	parentNode, errc := fs.lookup(parentPath)
	if errc != 0 {
		return errc, 0
	}

	name := filepath.Base(path)
	n := &node{
		fid:       "local_" + name + "_" + fmt.Sprint(time.Now().UnixNano()),
		parentFid: parentNode.fid,
		name:      name,
		size:      0,
		isFolder:  false,
		mtime:     time.Now(),
		isDirty:   true,
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

	fs.nodes.Store(path, n)
	if fs.cache != nil {
		fs.maybeSavePendingNodeLocked(path, n, true)
	}
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
	defer node.mu.Unlock()

	if fs.staging == nil || node.localPath == "" {
		return 0
	}

	// 1. 更新节点信息
	now := time.Now()
	node.isDirty = true
	node.mtime = now
	written, err := fs.staging.WriteAt(node.localPath, buff, ofst)
	if err != nil {
		return 0
	}
	if ofst+int64(written) > node.size {
		node.size = ofst + int64(written)
	}

	// 持久化节点元数据
	if fs.cache != nil {
		fs.maybeSavePendingNodeLocked(path, node, false)
	}

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
	if fs.staging == nil || n.localPath == "" {
		return -fuse.EIO
	}
	if err := fs.staging.Truncate(n.localPath, size); err != nil {
		return -fuse.EIO
	}
	if fs.cache != nil {
		fs.maybeSavePendingNodeLocked(path, n, true)
	}
	return 0
}

// Flush 刷新文件
func (fs *QryptFS) Flush(path string, fh uint64) (errc int) {
	node, errc := fs.lookup(path)
	if errc != 0 {
		return errc
	}

	node.mu.Lock()
	if node.isDirty && !node.syncQueued {
		fs.maybeSavePendingNodeLocked(path, node, true)
		node.syncQueued = true
		node.mu.Unlock()

		task := syncTask{node: node, path: path}
		select {
		case fs.uploadChan <- task:
			driver.Log.Printf("File %s queued for upload (fid=%s)\n", path, node.fid)
		default:
			driver.Log.Printf("Upload queue full, blocking for %s (fid=%s)\n", path, node.fid)
			fs.uploadChan <- task
		}
	} else {
		node.mu.Unlock()
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

func (fs *QryptFS) syncFile(path string, n *node) (err error) {
	startedAt := time.Now()
	stats := syncPerformanceSnapshot{Path: path}
	defer func() {
		stats.TotalDuration = time.Since(startedAt)
		fs.notifySyncFinish(stats, err)
	}()

	n.mu.Lock()
	if !n.isDirty {
		n.mu.Unlock()
		return nil
	}
	snapshotSize := n.size
	snapshotName := n.name
	snapshotMtime := n.mtime
	parentFid := n.parentFid
	localPath := n.localPath
	n.mu.Unlock()
	stats.SnapshotSize = snapshotSize

	if fs.staging == nil || fs.uploader == nil || localPath == "" {
		return fmt.Errorf("missing staging state for %s", path)
	}
	snapshotPath, err := fs.staging.Snapshot(localPath)
	if err != nil {
		return err
	}
	defer func() { _ = fs.staging.Remove(snapshotPath) }()

	driver.Log.Printf("Syncing file (Staged): %s (size %d, parentFid %s)\n", snapshotName, snapshotSize, parentFid)
	result, err := fs.uploader.Sync(uploadpkg.SyncRequest{
		Path:      path,
		Name:      snapshotName,
		ParentFid: parentFid,
		LocalPath: snapshotPath,
		PlainSize: snapshotSize,
	})
	stats.PreDuration = result.PreDuration
	stats.UpdateHashDuration = result.UpdateHashDuration
	stats.UploadPartDuration = result.UploadPartDuration
	stats.CommitDuration = result.CommitDuration
	stats.FinishDuration = result.FinishDuration
	stats.PartCount = result.PartCount
	stats.UploadedBytes = result.UploadedBytes
	if err != nil {
		return err
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	n.fid = result.Fid
	n.fileNonce = result.Nonce
	n.hasNonce = true
	n.encSize = result.EncryptedSize
	if n.mtime.Equal(snapshotMtime) {
		n.isDirty = false
	}
	if fs.cache != nil && !n.isDirty {
		_ = fs.cache.RemovePendingNode(path)
	}
	if !n.isDirty {
		if fs.staging != nil {
			_ = fs.staging.Remove(n.localPath)
		}
		n.localPath = ""
	}

	return nil
}

// Read 读取文件内容
func (fs *QryptFS) Read(path string, buff []byte, ofst int64, fh uint64) (n int) {
	driver.Log.Printf("[FUSE] Read: path=%s, len=%d, offset=%d, fh=%d\n", path, len(buff), ofst, fh)
	node, errc := fs.lookup(path)
	if errc != 0 {
		return 0
	}

	if ofst >= node.size {
		return 0
	}
	node.mu.RLock()
	localPath := node.localPath
	node.mu.RUnlock()
	if localPath != "" && fs.staging != nil {
		nRead, err := fs.staging.ReadAt(localPath, buff, ofst)
		if err == io.EOF {
			return nRead
		}
		if err == nil || nRead > 0 {
			return nRead
		}
	}

	// 计算涉及的分块
	startChunk := ofst / crypt.BlockDataSize
	endChunk := (ofst + int64(len(buff)) - 1) / crypt.BlockDataSize

	// 检测顺序读取：
	// 如果本次读取开始于上次读取结束的下一块，则视为顺序读取
	node.mu.Lock()
	if ofst == (node.lastReadBlock+1)*crypt.BlockDataSize {
		node.readSeqCount++
	} else if ofst != node.lastReadBlock*crypt.BlockDataSize { // 排除对同一块的重复读取（常见于 FUSE 对齐）
		node.readSeqCount = 0
	}
	node.lastReadBlock = endChunk
	seqCount := node.readSeqCount
	node.mu.Unlock()

	// 如果检测到连续顺序读取，触发批量预取
	if seqCount >= 2 {
		go fs.prefetch(node, uint64(endChunk+1))
	}

	totalRead := 0
	for i := startChunk; i <= endChunk; i++ {
		chunkData, err := fs.getDecryptedChunk(node, uint64(i))
		if err != nil {
			break
		}

		// 计算该分块中需要读取的范围
		chunkStart := int64(0)
		if i == startChunk {
			chunkStart = ofst % crypt.BlockDataSize
		}

		chunkEnd := int64(len(chunkData))
		remainingInBuff := int64(len(buff)) - int64(totalRead)
		if chunkEnd-chunkStart > remainingInBuff {
			chunkEnd = chunkStart + remainingInBuff
		}

		if chunkStart >= int64(len(chunkData)) {
			continue
		}

		nCopied := copy(buff[totalRead:], chunkData[chunkStart:chunkEnd])
		totalRead += nCopied

		if int64(totalRead) >= int64(len(buff)) {
			break
		}
	}

	return totalRead
}

func (fs *QryptFS) prefetch(n *node, startChunk uint64) {
	// 简单预取接下来的一批分块
	for i := uint64(0); i < FetchBatchBlocks/4; i++ {
		target := startChunk + i
		if int64(target)*crypt.BlockDataSize >= n.size {
			break
		}
		// 检查是否已在缓存
		if fs.cache != nil {
			if ok, _ := fs.cache.HasChunk(n.fid, int64(target)); ok {
				continue
			}
		}
		mKey := fmt.Sprintf("%s_%d", n.fid, target)
		if _, ok := fs.memCache.Load(mKey); ok {
			continue
		}

		// 触发异步加载
		go func(idx uint64) {
			_, _ = fs.getDecryptedChunk(n, idx)
		}(target)
	}
}

func (fs *QryptFS) getDecryptedChunk(n *node, idx uint64) ([]byte, error) {
	mKey := fmt.Sprintf("%s_%d", n.fid, idx)

	// 1. 内存二级缓存
	if v, ok := fs.memCache.Load(mKey); ok {
		return v.([]byte), nil
	}

	// 2. 本地持久化缓存
	if fs.cache != nil {
		data, err := fs.cache.GetChunk(n.fid, int64(idx))
		if err == nil && len(data) > 0 {
			fs.memCache.Store(mKey, data)
			return data, nil
		}
	}

	// 3. 远程获取并解密
	if strings.HasPrefix(n.fid, "local_") {
		return make([]byte, 0, crypt.BlockDataSize), nil
	}

	// 4. 批量获取策略
	batchIdx := idx / FetchBatchBlocks
	batchKey := fmt.Sprintf("%s_batch_%d", n.fid, batchIdx)

	// 使用 WaitGroup/Channel 合并同一批次的并发请求
	actual, loaded := fs.fetching.LoadOrStore(batchKey, make(chan struct{}))
	if loaded {
		// 等待已有请求完成
		<-actual.(chan struct{})
		// 等待结束后，直接尝试从缓存读取，不再递归以防死锁
		if v, ok := fs.memCache.Load(mKey); ok {
			return v.([]byte), nil
		}
		if fs.cache != nil {
			data, err := fs.cache.GetChunk(n.fid, int64(idx))
			if err == nil && len(data) > 0 {
				return data, nil
			}
		}
		return nil, fmt.Errorf("data not found after concurrent fetch")
	}

	// 执行批量下载
	defer func() {
		close(actual.(chan struct{}))
		fs.fetching.Delete(batchKey)
	}()

	err := fs.fetchBatch(n, batchIdx)
	if err != nil {
		return nil, err
	}

	// 此时缓存中应该已经有了
	if v, ok := fs.memCache.Load(mKey); ok {
		return v.([]byte), nil
	}
	// 特殊情况：如果请求的索引超出了文件范围（fetchBatch 没读到），返回空
	return make([]byte, 0, crypt.BlockDataSize), nil
}

func (fs *QryptFS) fetchBatch(n *node, batchIdx uint64) error {
	url, err := fs.driver.GetDownloadURL(n.fid)
	if err != nil {
		return err
	}

	startBlock := batchIdx * FetchBatchBlocks
	endBlock := startBlock + FetchBatchBlocks - 1

	pStart := int64(crypt.FileHeaderSize) + int64(startBlock)*int64(crypt.BlockSize)
	if pStart >= n.encSize {
		// 如果起始位置已经超过文件大小，说明可能是很小的文件或者逻辑错误
		// 尝试从头开始下载，以获取完整上下文
		pStart = 0
	}
	pEnd := pStart + int64(FetchBatchBlocks)*int64(crypt.BlockSize) - 1
	if pStart == 0 {
		pEnd += int64(crypt.FileHeaderSize)
	}
	if pEnd >= n.encSize {
		pEnd = n.encSize - 1
	}

	driver.Log.Printf("Batch Fetch: '%s' Blocks %d-%d\n", n.name, startBlock, endBlock)

	rc, err := fs.driver.DownloadChunk(url, pStart, pEnd)
	if err != nil {
		// 如果是 416 (Requested Range Not Satisfiable)，说明文件可能比预想的小（例如 0 字节），返回空数据而不报错
		if strings.Contains(err.Error(), "416") {
			return nil
		}
		return err
	}
	defer rc.Close()

	if !n.hasNonce {
		hrc, err := fs.driver.DownloadChunk(url, 0, int64(crypt.FileHeaderSize)-1)
		if err == nil {
			header := make([]byte, crypt.FileHeaderSize)
			_, errRead := io.ReadFull(hrc, header)
			hrc.Close()
			if errRead == nil && string(header[:len(crypt.FileMagic)]) == crypt.FileMagic {
				n.mu.Lock()
				copy(n.fileNonce[:], header[len(crypt.FileMagic):])
				n.hasNonce = true
				n.mu.Unlock()
			}
		}
	}

	if pStart == 0 {
		// 跳过 Header
		header := make([]byte, crypt.FileHeaderSize)
		_, errHeader := io.ReadFull(rc, header)
		if errHeader == nil && string(header[:len(crypt.FileMagic)]) == crypt.FileMagic {
			if !n.hasNonce {
				n.mu.Lock()
				copy(n.fileNonce[:], header[len(crypt.FileMagic):])
				n.hasNonce = true
				n.mu.Unlock()
			}
		}
	}

	// 流式解析并存入缓存
	for i := startBlock; i <= endBlock; i++ {
		encBlock := make([]byte, crypt.BlockSize)
		nRead, err := io.ReadFull(rc, encBlock)
		if err != nil {
			if err == io.EOF {
				break
			}
			if err == io.ErrUnexpectedEOF {
				if nRead > crypt.BlockHeaderSize {
					// 这是一个残留块，继续处理
					encBlock = encBlock[:nRead]
				} else {
					break
				}
			} else {
				return err
			}
		}

		n.mu.RLock()
		nonce := n.fileNonce
		n.mu.RUnlock()

		decBlock, err := fs.cipher.DecryptBlock(encBlock, uint64(i), nonce)
		if err != nil {
			return fmt.Errorf("decryption failed for block %d: %v", i, err)
		}

		// 存入持久化缓存
		if fs.cache != nil {
			_ = fs.cache.PutChunk(n.fid, int64(i), decBlock, false)
		}
		// 存入内存缓存
		mKey := fmt.Sprintf("%s_%d", n.fid, i)
		fs.memCache.Store(mKey, decBlock)
	}

	return nil
}

// Statfs 返回文件系统统计信息
func (fs *QryptFS) Statfs(path string, stat *fuse.Statfs_t) (errc int) {
	stat.Bsize = 4096
	stat.Frsize = 4096
	stat.Blocks = 1024 * 1024 * 1024 // 虚拟 4TB
	stat.Bfree = 1024 * 1024 * 512
	stat.Bavail = 1024 * 1024 * 512
	return 0
}
