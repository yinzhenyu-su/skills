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

	memCache, _ := lru.New[string, []byte](MemCacheMaxEntries)

	fs := &QryptFS{
		driver:     d,
		cache:      c,
		rootFid:    rootFid,
		cipher:     cipher,
		uploadChan: make(chan syncTask, 1000), // 允许排队 1000 个文件
		staging:    stagingStore,
		memCache:   memCache,
	}
	if stagingStore != nil {
		fs.uploader = uploadpkg.NewManager(d, cipher, stagingStore)
	}
	fs.storeNode("/", &node{fid: rootFid, currentPath: "/", isFolder: true, mtime: time.Now(), lastReadBlock: -1})

	// 启动后台上传工作协程 (限制并发为 3)
	for i := 0; i < 3; i++ {
		go fs.uploadWorker()
	}

	// 恢复上次未完成的任务
	fs.recoverDirtyFiles()
	fs.recoverPendingOps()

	return fs
}
