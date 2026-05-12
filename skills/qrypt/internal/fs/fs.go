package fs

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/golang-lru/v2"
	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/quark"
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

type QryptFS struct {
	fuse.FileSystemBase

	fileSvc   *quark.FileService
	manageSvc *quark.ManageService
	cacheSvc  *quark.CacheService
	cipher    *crypt.RcloneCipher
	cacheMgr  *cache.CacheManager
	staging   *staging.Store

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
	metadataOpChan chan metadataTask

	shuttingDown int32
	workerWg     sync.WaitGroup
	maxRetries   int
}

type FSOptions struct {
	MaxRetries        int
	ConcurrentUploads int
	MemCacheSizeMB    int
}

func NewFS(
	fileSvc *quark.FileService,
	manageSvc *quark.ManageService,
	cacheSvc *quark.CacheService,
	cipher *crypt.RcloneCipher,
	cacheMgr *cache.CacheManager,
	quarkClient *quark.Client,
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

	uploadSvc := quark.NewUploadService(quarkClient)
	uploader := syncpkg.NewUploader(fileSvc, manageSvc, uploadSvc, cacheSvc, cipher)

	fs := &QryptFS{
		fileSvc:        fileSvc,
		manageSvc:      manageSvc,
		cacheSvc:       cacheSvc,
		cipher:         cipher,
		cacheMgr:       cacheMgr,
		staging:        stg,
		rootFid:        rootFid,
		uploader:       uploader,
		uploadChan:     make(chan syncTask, 1000),
		metadataOpChan: make(chan metadataTask, 100000),
		prefetchSem:    make(chan struct{}, 30),
		lruStop:        make(chan struct{}),
		memCache:       memCache,
		maxRetries:     maxRetries,
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

	go fs.lruEvictionLoop()
	if fs.cacheMgr != nil {
		fs.cacheMgr.MaintenanceStart()
	}

	return fs
}

func (fs *QryptFS) IsShuttingDown() bool {
	return atomic.LoadInt32(&fs.shuttingDown) == 1
}

func (fs *QryptFS) Shutdown() {
	atomic.StoreInt32(&fs.shuttingDown, 1)
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
	case <-time.After(30 * time.Second):
	}
}
