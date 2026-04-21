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
}

// NewQryptFS 创建新的文件系统实例
// rootDirName is the plaintext name of the mount root directory (e.g., "MyDrive").
// Used to recreate the root directory if it's deleted externally.
func NewQryptFS(d *driver.QuarkDriver, c *cache.CacheManager, rootFid string, rootDirName string, cipher *crypt.RcloneCipher, vfsCfg ...QryptFSConfig) *QryptFS {
	var stagingStore *staging.Store
	if c != nil {
		s, err := staging.NewStore(c.StagingDir())
		if err != nil {
			driver.Log.Printf("Failed to initialize staging store: %v\n", err)
		} else {
			stagingStore = s
		}
	}

	memCache, _ := lru.New[string, []byte](MemCacheMaxEntries)

	// Apply config defaults
	maxRetries := maxAutoRetryAttempts
	concurrentUploads := 3
	if len(vfsCfg) > 0 {
		if vfsCfg[0].MaxRetries > 0 {
			maxRetries = vfsCfg[0].MaxRetries
		}
		if vfsCfg[0].ConcurrentUploads > 0 {
			concurrentUploads = vfsCfg[0].ConcurrentUploads
		}
	}

	fs := &QryptFS{
		driver:          d,
		cache:           c,
		rootFid:         rootFid,
		cipher:          cipher,
		uploadChan:      make(chan syncTask, 1000), // 允许排队 1000 个文件
		staging:         stagingStore,
		memCache:        memCache,
		maxRetries:      maxRetries,
	}
	if stagingStore != nil {
		fs.uploader = uploadpkg.NewManager(d, cipher, stagingStore)
	}
	fs.storeNode("/", &node{fid: rootFid, name: rootDirName, parentFid: "0", currentPath: "/", isFolder: true, mtime: time.Now(), lastReadBlock: -1})

	// 启动后台上传工作协程
	for i := 0; i < concurrentUploads; i++ {
		go fs.uploadWorker()
	}

	// 恢复上次未完成的任务
	fs.recoverDirtyFiles()
	fs.recoverPendingOps()

	return fs
}
