package vfs

import (
	"errors"
	"sync"
	"sync/atomic"
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
	// FetchBatchSizeMB 定义了一次批量下载的大小 (单位: MB)
	FetchBatchSizeMB = 32
	// FetchBatchBlocks 根据 MB 自动计算块数 (32MB / 64KB = 512)
	FetchBatchBlocks = (FetchBatchSizeMB * 1024) / 64

	// MemCacheSizeMB 内存缓存总体上限 (单位: MB)
	MemCacheSizeMB = 512
	// MemCacheMaxEntries 根据 MB 自动计算条目数 (512MB / 64KB = 8192)
	MemCacheMaxEntries = (MemCacheSizeMB * 1024) / 64

	maxAutoRetryAttempts    = 5
	pendingNodeSaveInterval = 250 * time.Millisecond
	pendingNodeSaveSizeStep = 1 * 1024 * 1024
)

var (
	// MetadataTTL 定义了元数据缓存的有效期 (15s)
	MetadataTTL = 15 * time.Second
)

var (
	errNonRetryableSync = errors.New("non-retryable sync error")
	errDirGone          = errors.New("parent directory deleted during upload")
)

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
	lastRemoteCheck time.Time // 上次远程列表检查的时间（用于 readdir 同步）
	lastReadBlock   int64     // 上次读取的块索引
	readSeqCount    int       // 连续顺序读取的块数
	lastPendingSave time.Time
	lastPendingSize   int64
	source            string            // "remote" | "local" | "merged" — 文件来源
	expectedFid       string            // 预期服务端返回的 FID（用于抵御索引延迟导致的冲突）
	uploadedFid       string            // 上次成功上传后的远程 FID（用于 FID 直接替换，绕过 ListFiles 索引延迟）
	writeInFlight     int32             // 正在进行的 Write 操作计数（atomic）
	cancelled         int32             // 1 = 已取消（被删除），原子操作
	children          map[string]*node // 子节点缓存 (name -> *node), 避免 O(N) 扫描
	mu                sync.RWMutex
}

// addWriteInFlight 原子递增写入计数
func (n *node) addWriteInFlight() { atomic.AddInt32(&n.writeInFlight, 1) }

// doneWriteInFlight 原子递减写入计数
func (n *node) doneWriteInFlight() { atomic.AddInt32(&n.writeInFlight, -1) }

// hasWriteInFlight 检查是否有进行中的写入
func (n *node) hasWriteInFlight() bool { return atomic.LoadInt32(&n.writeInFlight) > 0 }

// cancel 标记节点已取消（被删除），用于拦截待上传的任务
func (n *node) cancel() { atomic.StoreInt32(&n.cancelled, 1) }

// isCancelled 检查节点是否已被取消
func (n *node) isCancelled() bool { return atomic.LoadInt32(&n.cancelled) == 1 }

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

type metadataTask struct {
	opType string
	logID  int64
	path   string
	node   *node
	fids   []string
}

type deletionState struct {
	parentFid string
	path      string // 记录删除时的路径，用于证据确认后清理 deletingPaths
	apiDone   bool
}

// QryptFS 实现了 fuse.FileSystem 接口
type QryptFS struct {
	fuse.FileSystemBase
	driver          *driver.QuarkDriver
	cache           *cache.CacheManager
	cipher          *crypt.RcloneCipher
	rootFid         string
	nodes           sync.Map // path -> *node
	fidNodes        sync.Map // fid -> *node (用于快速反查)
	fetching        sync.Map // batchKey -> chan struct{} (用于合并请求)
	merging         sync.Map // fid -> chan struct{} (用于合并 MergeRemoteChanges 操作)
	deletingPaths   sync.Map // path -> struct{} (正在删除的目录)
	activeDeletions sync.Map // fid -> *deletionState (正在执行或等待同步的删除任务)
	deletionsByParent sync.Map // parentFid -> *sync.Map (辅助索引：快速找到某个目录下的所有子墓碑)
	memCache        *lru.Cache[string, []byte] // fid_idx -> []byte (有界内存二级缓存)
	uploadChan      chan syncTask
	metadataOpChan  chan metadataTask // Background metadata task queue
	opsLogChan      chan metadataTask // New: Buffered channel for ops log entries
	prefetchSem     chan struct{}     // Limit directory prefetch concurrency
	syncing         sync.Map          // *node -> struct{} (防止并发同步同一节点)
	retryState      sync.Map          // *node -> int (基于节点的自动重试次数)
	dirGoneRetry    sync.Map          // *node -> int (目录被删除重试次数)
	syncObserver    syncObserver
	staging         *staging.Store
	uploader        *uploadpkg.Manager
	maxRetries      int
	shuttingDown    int32 // 1 = shutdown in progress, guards retry goroutines from writing to closed channel
}
