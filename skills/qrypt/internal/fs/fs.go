//go:build !nofuse

package fs

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/golang-lru/v2"
	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
	"github.com/yinzhenyu/skills/qrypt/internal/log"
	"github.com/yinzhenyu/skills/qrypt/internal/staging"
	syncpkg "github.com/yinzhenyu/skills/qrypt/internal/sync"
)

const (
	FetchBatchSizeMB    = 32
	FetchBatchBlocks    = (FetchBatchSizeMB * 1024) / 64
	MemCacheSizeMB      = 512
	MemCacheMaxEntries  = (MemCacheSizeMB * 1024) / 64
	MaxAutoRetries      = 5
	MetadataTTL         = 15 * time.Second
)

// UploadQueue allows the daemon to provide a shared worker pool for VFS uploads.
// When set, enqueueSyncDelay uses it instead of the internal uploadChan.
type UploadQueue interface {
	Submit(job func(ctx context.Context) error) bool
}

var errNonRetryableSync = errors.New("non-retryable sync error")
var errDirGone = errors.New("parent directory deleted during upload")

const (
	pendingNodeSaveInterval = 250 * time.Millisecond
	pendingNodeSaveSizeStep = 1 * 1024 * 1024
)

type syncTask struct {
	node    *Node
}

type metadataTask struct {
	opType string
	path   string
	node   *Node
	fids   []string
}

type deletionState struct {
	parentFid string
	path      string
	apiDone   bool
}

// SetUploadQueue replaces the internal uploadChan with an external queue.
// Call before any uploads are enqueued (e.g., right after NewFS).
func (fs *QryptFS) SetUploadQueue(q UploadQueue) {
	fs.uploadQueue = q
}

type QryptFS struct {
	fuse.FileSystemBase

	drv     drive.Driver
	cipher  *crypt.RcloneCipher
	cacheMgr *cache.CacheManager
	staging *staging.Store

	rootFid string
	nodes   sync.Map
	fidNodes sync.Map

	fetchingFiles   sync.Map
	fetchingChunks  sync.Map
	merging         sync.Map
	prefetchSem     chan struct{}
	lruStop         chan struct{}
	deletingPaths   sync.Map
	activeDeletions sync.Map
	deletionsByParent sync.Map
	retryState      sync.Map

	memCache *lru.Cache[string, []byte]

	uploader      *syncpkg.Uploader
	uploadChan    chan syncTask
	uploadQueue   UploadQueue // optional: replaces uploadChan when set
	metadataOpChan chan metadataTask

	shuttingDown    int32
	workerWg        sync.WaitGroup
	maxRetries      int
	writeBackDelay  time.Duration

	syncDelayMu   sync.Mutex
	syncTimers    map[string]*time.Timer // path → resettable upload timer
}

type FSOptions struct {
	MaxRetries        int
	ConcurrentUploads int
	MemCacheSizeMB    int
	WriteBackTimeout  time.Duration // 0 means immediate
}

func NewFS(
	drv drive.Driver,
	cipher *crypt.RcloneCipher,
	cacheMgr *cache.CacheManager,
	rootFid string,
	opts FSOptions,
) *QryptFS {
	maxRetries := MaxAutoRetries
	concUploads := 3
	memSize := MemCacheSizeMB

	if opts.MaxRetries > 0 {
		maxRetries = opts.MaxRetries
	}
	if opts.ConcurrentUploads > 0 {
		concUploads = opts.ConcurrentUploads
	}
	if opts.MemCacheSizeMB > 0 {
		memSize = opts.MemCacheSizeMB
	}

	memCacheMax := (memSize * 1024) / 64
	memCache, _ := lru.New[string, []byte](memCacheMax)

	var stg *staging.Store
	if cacheMgr != nil {
		stg = cacheMgr.Staging()
	}

	uploader := syncpkg.NewUploader(drv, cipher)

	fs := &QryptFS{
		drv:             drv,
		cipher:          cipher,
		cacheMgr:        cacheMgr,
		staging:         stg,
		rootFid:         rootFid,
		uploader:        uploader,
		uploadChan:      make(chan syncTask, 1000),
		metadataOpChan:  make(chan metadataTask, 100000),
		prefetchSem:     make(chan struct{}, 30),
		lruStop:         make(chan struct{}),
		memCache:        memCache,
		maxRetries:      maxRetries,
		writeBackDelay:  opts.WriteBackTimeout,
		syncTimers:      make(map[string]*time.Timer),
	}

	rootName := ""
	rootNode := newNode(rootFid, "0", rootName, "/", true)
	rootNode.source = "remote"
	rootNode.mtime = time.Now()
	fs.storeNode("/", rootNode)

	for i := 0; i < concUploads; i++ {
		fs.workerWg.Add(1)
		go fs.uploadWorker()
	}
	fs.workerWg.Add(1)
	go fs.metadataWorker()

	fs.recoverDirtyFiles()
	fs.replayOpsLog()

	if fs.staging != nil {
		activeFids := make(map[string]bool)
		fs.nodes.Range(func(key, value interface{}) bool {
			n := value.(*Node)
			n.mu.RLock()
			activeFids[n.fid] = true
			n.mu.RUnlock()
			return true
		})
		cleaned, err := fs.staging.CleanupOrphanedStagingFiles(activeFids)
		if err != nil {
			log.L.Warnf("staging cleanup: %v\n", err)
		} else if len(cleaned) > 0 {
			log.L.Infof("staging cleanup: removed %d orphaned staging files\n", len(cleaned))
		}
	}

	go fs.lruEvictionLoop()
	if fs.cacheMgr != nil {
		fs.cacheMgr.MaintenanceStart()
	}

	return fs
}

// InvalidateDirCache removes a path from the in-memory node tree,
// causing the next lookup to re-fetch from the remote. This is called
// by the daemon's CacheInvalidator when external uploads complete.
func (fs *QryptFS) InvalidateDirCache(path string) {
	if v, ok := fs.nodes.Load(path); ok {
		n := v.(*Node)
		n.mu.RLock()
		fid := n.fid
		n.mu.RUnlock()
		if fid != "" && !strings.HasPrefix(fid, "local_") {
			fs.fidNodes.Delete(fid)
		}
		fs.nodes.Delete(path)
		log.L.Debugf("Invalidated cache for %s\n", path)
	}
}

func (fs *QryptFS) IsShuttingDown() bool {
	return atomic.LoadInt32(&fs.shuttingDown) == 1
}

func (fs *QryptFS) Shutdown() {
	log.L.Infof("Shutdown: starting graceful shutdown...\n")
	atomic.StoreInt32(&fs.shuttingDown, 1)

	// Flush all pending staging page buffers to disk so no data is lost
	// on restart. Do this before closing channels — workers may still be
	// running and can safely operate on the staging store.
	if fs.staging != nil {
		fs.nodes.Range(func(key, value interface{}) bool {
			n := value.(*Node)
			n.mu.RLock()
			localPath := n.localPath
			n.mu.RUnlock()
			if localPath != "" {
				if err := fs.staging.Sync(localPath); err != nil {
					log.L.Warnf("Shutdown: staging.Sync %s: %v\n", localPath, err)
				}
			}
			return true
		})
	}

	// Compact pending journal so restart has clean state.
	if fs.cacheMgr != nil {
		if err := fs.cacheMgr.Close(); err != nil {
			log.L.Warnf("Shutdown: cacheMgr.Close: %v\n", err)
		}
	}

	// Signal workers to drain remaining tasks.
	close(fs.lruStop)
	close(fs.uploadChan)
	close(fs.metadataOpChan)

	done := make(chan struct{})
	go func() {
		fs.workerWg.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.L.Infof("Shutdown: all workers finished\n")
	case <-time.After(30 * time.Second):
		log.L.Warnf("Shutdown: workers did not finish within 30s\n")
	}
}
