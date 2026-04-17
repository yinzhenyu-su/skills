package vfs

import (
	"errors"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2"
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
	// MemCacheMaxEntries 默认内存缓存条目数 (≈32MB，够4个prefetch batch)
	MemCacheMaxEntries = 512
)

var (
	// MetadataTTL 定义了元数据缓存的有效期 (60s)
	MetadataTTL = 60 * time.Second
)

var errNonRetryableSync = errors.New("non-retryable sync error")

type node struct {
	fid             string
	parentFid       string // 父目录 FID
	name            string // 明文名称
	size            int64  // 原始明文大小
	encSize         int64  // 网盘上的加密大小
	localPath       string // 本地 staging 文件
	currentPath     string
	isFolder        bool
	mtime           time.Time // 修改时间
	fileNonce       [24]byte
	hasNonce        bool
	isDirty         bool      // 是否有未同步的修改
	syncQueued      bool      // 是否已在同步队列中
	baseServerMtime int64     // 上次同步成功的服务端修改时间 (ms)
	baseServerSize  int64     // 上次同步成功的服务端明文大小
	lastMetadataCheck time.Time // 上次从服务器拉取元数据的时间
	lastUploadTime  time.Time // 上次上传完成的时间（用于防止 API 索引延迟导致误删）
	lastReadBlock   int64     // 上次读取的块索引
	readSeqCount    int       // 连续顺序读取的块数
	lastPendingSave time.Time
	lastPendingSize   int64
	children          map[string]*node // 子节点缓存 (name -> *node), 避免 O(N) 扫描
	mu                sync.RWMutex
}

type syncTask struct {
	node     *node
	opsLogID int64 // 关联的日志记录 ID
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
	fidNodes     sync.Map // fid -> *node (用于快速反查)
	fetching     sync.Map // batchKey -> chan struct{} (用于合并请求)
	memCache     *lru.Cache[string, []byte] // fid_idx -> []byte (有界内存二级缓存)
	uploadChan   chan syncTask
	syncing      sync.Map // *node -> struct{} (防止并发同步同一节点)
	retryState   sync.Map // *node -> int (基于节点的自动重试次数)
	syncObserver syncObserver
	staging      *staging.Store
	uploader     *uploadpkg.Manager
}
