package vfs

import (
	"fmt"
	"io"
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
	FetchBatchBlocks = 128
)

type node struct {
	fid       string
	name      string    // 明文名称
	size      int64     // 原始明文大小
	encSize   int64     // 网盘上的加密大小
	isFolder  bool
	mtime     time.Time // 修改时间
	fileNonce [24]byte
	hasNonce  bool
}

// QryptFS 实现了 fuse.FileSystem 接口
type QryptFS struct {
	fuse.FileSystemBase
	driver   *driver.QuarkDriver
	cache    *cache.CacheManager
	cipher   *crypt.RcloneCipher
	rootFid  string
	nodes    sync.Map // path -> *node
	fetching sync.Map // batchKey -> chan struct{} (用于合并请求)
	memCache sync.Map // fid_idx -> []byte (内存二级缓存)
}

// NewQryptFS 创建新的文件系统实例
func NewQryptFS(d *driver.QuarkDriver, c *cache.CacheManager, rootFid string, cipher *crypt.RcloneCipher) *QryptFS {
	fs := &QryptFS{
		driver:  d,
		cache:   c,
		rootFid: rootFid,
		cipher:  cipher,
	}
	fs.nodes.Store("/", &node{fid: rootFid, isFolder: true, mtime: time.Now()})
	return fs
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

	if n.isFolder {
		stat.Mode = fuse.S_IFDIR | 0755
	} else {
		stat.Mode = fuse.S_IFREG | 0644
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
		decName, err := fs.cipher.DecryptSegment(f.FileName)
		if err != nil {
			continue
		}

		stat := &fuse.Stat_t{}
		decSize, _ := fs.cipher.DecryptedSize(f.Size)
		modTime := f.ModTime()
		if f.IsDir() {
			stat.Mode = fuse.S_IFDIR | 0755
		} else {
			stat.Mode = fuse.S_IFREG | 0644
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

// Open 打开文件
func (fs *QryptFS) Open(path string, flags int) (errc int, fh uint64) {
	_, errc = fs.lookup(path)
	return errc, 0
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

	// 启动后台预取：如果读到当前 batch 的末尾，预取下一个 batch
	nextBatchIdx := (uint64(endChunk) / FetchBatchBlocks) + 1
	go fs.prefetchBatch(node, nextBatchIdx)

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

// prefetchBatch 异步下载下一个 8MB 批次
func (fs *QryptFS) prefetchBatch(n *node, batchIdx uint64) {
	// 检查是否超出文件范围
	if int64(batchIdx)*FetchBatchBlocks*crypt.BlockDataSize >= n.size {
		return
	}
	// 尝试获取该批次的第一个块，会自动触发整个批次的下载
	_, _ = fs.getDecryptedChunk(n, batchIdx*FetchBatchBlocks)
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
			_ = fs.cache.PutChunk(n.fid, int64(i), plaintext)
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
