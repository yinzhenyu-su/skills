package vfs

import (
	"time"

	"github.com/hashicorp/golang-lru/v2"
	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/driver"
	"github.com/yinzhenyu/skills/qrypt/internal/staging"
	uploadpkg "github.com/yinzhenyu/skills/qrypt/internal/upload"
)

// QryptFSConfig contains VFS configuration options
type QryptFSConfig struct {
	MaxRetries        int // max retry attempts for failed uploads (0 = use default 5)
	ConcurrentUploads int // number of concurrent upload workers (0 = use default 3)
	MemCacheSizeMB    int // memory cache size in MB (0 = use default 512)
}

// NewQryptFS 创建新的文件系统实例
// rootDirName is the plaintext name of the mount root directory (e.g., "MyDrive").
// Used to recreate the root directory if it's deleted externally.
func NewQryptFS(d *driver.QuarkDriver, c *cache.CacheManager, rootFid string, rootDirName string, cipher *crypt.RcloneCipher, vfsCfg ...QryptFSConfig) *QryptFS {
	var stagingStore *staging.Store
	if c != nil {
		s, err := staging.NewStore(c.StagingDir())
		if err != nil {
			driver.Log.Errorf("Failed to initialize staging store: %v\n", err)
		} else {
			stagingStore = s
		}
	}

	// Apply config defaults
	maxRetries := maxAutoRetryAttempts
	concurrentUploads := 3
	memCacheSizeMB := MemCacheSizeMB
	if len(vfsCfg) > 0 {
		if vfsCfg[0].MaxRetries > 0 {
			maxRetries = vfsCfg[0].MaxRetries
		}
		if vfsCfg[0].ConcurrentUploads > 0 {
			concurrentUploads = vfsCfg[0].ConcurrentUploads
		}
		if vfsCfg[0].MemCacheSizeMB > 0 {
			memCacheSizeMB = vfsCfg[0].MemCacheSizeMB
		}
	}

	// Recalculate memCache entries based on configured size
	memCacheMaxEntries := (memCacheSizeMB * 1024) / 64
	memCache, _ := lru.New[string, []byte](memCacheMaxEntries)

	fs := &QryptFS{
		driver:         d,
		cache:          c,
		rootFid:        rootFid,
		cipher:         cipher,
		uploadChan:     make(chan syncTask, 1000), // 允许排队 1000 个文件
		metadataOpChan: make(chan metadataTask, 100000), // 增加到 10w，防止批量删除卡死
		opsLogChan:     make(chan metadataTask, 100000), // 用于异步写入 ops_log
		prefetchSem:    make(chan struct{}, 30),        // 限制并发预取数量
		staging:        stagingStore,
		memCache:       memCache,
		maxRetries:     maxRetries,
	}
	if stagingStore != nil {
		fs.uploader = uploadpkg.NewManager(d, cipher, stagingStore)
	}
	fs.storeNode("/", &node{fid: rootFid, name: rootDirName, parentFid: "0", currentPath: "/", isFolder: true, mtime: time.Now(), lastReadBlock: -1})

	// 启动后台上传工作协程
	for i := 0; i < concurrentUploads; i++ {
		go fs.uploadWorker()
	}

	// 启动后台元数据工作协程 (改为 1 个以提升批量效率并减少 DB 锁竞争)
	go fs.metadataWorker()

	// 启动异步日志工作协程
	go fs.opsLogWorker()

	fs.recoverDirtyFiles()
	fs.recoverPendingOps()

	return fs
}

// Shutdown 优雅关闭：停止接收新任务，等待正在处理的上传完成
func (fs *QryptFS) Shutdown() {
	driver.Log.Info("Shutdown: closing upload channel, waiting for inflight tasks...\n")

	// 关闭上传通道，worker 会在处理完当前任务后退出
	close(fs.uploadChan)

	// 关闭元数据操作通道
	close(fs.metadataOpChan)

	// 关闭 ops_log 通道
	close(fs.opsLogChan)

	driver.Log.Info("Shutdown complete\n")
}
