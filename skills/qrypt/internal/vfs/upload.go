package vfs

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
)

type uploadInfo struct {
	ParentID      string
	Name          string
	Size          int64
	OldFid        string
	StagingReader func() (io.ReadCloser, error)
}

// uploadAttempt captures the full identity and context of a scheduled upload.
// It is created at MarkDirty time and threaded through the entire upload
// lifecycle so that OnUploadComplete can detect stale commits.
type uploadAttempt struct {
	Path          string
	Fid           string
	Generation    uint64
	Node          *VFSNode
	OldFid        string
	ParentID      string
	Name          string
	Size          int64
	StagingReader func() (io.ReadCloser, error)
}

// timerFactory is a pluggable timer constructor for test injection.
type timerFactory func(d time.Duration, f func()) *time.Timer

type Uploader interface {
	Upload(ctx context.Context, parentID, name string, size int64, body io.Reader) (drivers.Entry, error)
}

type UploadManager struct {
	executor   Uploader
	observer   UploadObserver
	writeBack  time.Duration
	maxRetries int
	newTimer   timerFactory

	mu         sync.Mutex
	timers     map[string]*time.Timer
	retryCount map[string]int
	infos      map[string]*uploadInfo
	attempts   map[string]*uploadAttempt // identity snapshot at schedule time
	inFlight   map[string]bool           // paths currently being uploaded

	coalescer *OperationCoalescer // optional, set via SetCoalescer

	// concurrency control
	sem            chan struct{} // buffered channel semaphore, cap = limit
	concurrentCount int32        // atomic: current in-flight uploads
	peakConcurrent  int32        // atomic: observed peak (for tests)

	// per-upload context cancellation
	rootCtx    context.Context
	rootCancel context.CancelFunc
	cancelCtx  map[string]context.CancelFunc

	// shutdown flag — once set, no new uploads are scheduled
	shutdown atomic.Bool
}

func NewUploadManager(executor Uploader, writeBack time.Duration, observer UploadObserver) *UploadManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &UploadManager{
		executor:   executor,
		observer:   observer,
		writeBack:  writeBack,
		maxRetries: 5,
		newTimer:   time.AfterFunc,
		timers:     make(map[string]*time.Timer),
		retryCount: make(map[string]int),
		infos:      make(map[string]*uploadInfo),
		attempts:   make(map[string]*uploadAttempt),
		inFlight:   make(map[string]bool),
		sem:        make(chan struct{}, 4), // default concurrency limit
		rootCtx:    ctx,
		rootCancel: cancel,
		cancelCtx:  make(map[string]context.CancelFunc),
	}
}

// SetMaxRetries overrides the maximum number of upload attempts (for testing).
func (um *UploadManager) SetMaxRetries(n int) {
	um.maxRetries = n
}

// SetTimerFactory replaces the timer constructor (for testing).
func (um *UploadManager) SetTimerFactory(f timerFactory) {
	um.newTimer = f
}

// SetCoalescer attaches an optional OperationCoalescer for lifecycle
// tracking. The coalescer receives MarkUploadStarted / MarkUploadCommitted
// calls as uploads progress.
func (um *UploadManager) SetCoalescer(c *OperationCoalescer) {
	um.coalescer = c
}

// SetConcurrencyLimit sets the maximum number of simultaneous Upload calls.
// Must be called before any MarkDirty calls (not goroutine-safe after first use).
// If n <= 0, the effective limit is 4.
func (um *UploadManager) SetConcurrencyLimit(n int) {
	if n <= 0 {
		n = 4
	}
	um.sem = make(chan struct{}, n)
}

// PeakConcurrentUploads returns the peak observed concurrent upload count
// since construction. Used in tests to verify concurrency bounds.
func (um *UploadManager) PeakConcurrentUploads() int {
	return int(atomic.LoadInt32(&um.peakConcurrent))
}

func (um *UploadManager) MarkDirty(path string, parentID, name string, size int64, oldFid string, stagingReader func() (io.ReadCloser, error), node *VFSNode) {
	um.mu.Lock()
	defer um.mu.Unlock()

	um.infos[path] = &uploadInfo{
		ParentID:      parentID,
		Name:          name,
		Size:          size,
		OldFid:        oldFid,
		StagingReader: stagingReader,
	}

	// Build the uploadAttempt identity snapshot at schedule time.
	fid := ""
	if node != nil {
		fid = node.FID()
	}
	var gen uint64
	if um.coalescer != nil && fid != "" {
		if intent := um.coalescer.findIntent(fid); intent != nil && intent.Upload != nil {
			gen = intent.Upload.Generation
		}
	}
	um.attempts[path] = &uploadAttempt{
		Path:          path,
		Fid:           fid,
		Generation:    gen,
		Node:          node,
		OldFid:        oldFid,
		ParentID:      parentID,
		Name:          name,
		Size:          size,
		StagingReader: stagingReader,
	}

	// If already uploading, just update the info — the in-flight upload
	// will pick up the latest version when it completes.
	if um.inFlight[path] {
		if t, ok := um.timers[path]; ok {
			t.Stop()
			delete(um.timers, path)
		}
		return
	}

	if t, ok := um.timers[path]; ok {
		t.Stop()
	}

	if um.shutdown.Load() {
		// After Shutdown(), record state but don't schedule new uploads.
		return
	}

	delay := um.writeBack
	if delay <= 0 {
		delay = 200 * time.Millisecond
	}
	um.timers[path] = um.newTimer(delay, func() {
		um.mu.Lock()
		if um.inFlight[path] {
			// Another upload is already running; defer to its completion check.
			um.mu.Unlock()
			return
		}
		info, ok := um.infos[path]
		if !ok {
			um.mu.Unlock()
			return
		}
		attempt := um.attempts[path]
		delete(um.timers, path)
		um.inFlight[path] = true
		um.mu.Unlock()
		um.enqueueUpload(path, info, attempt)
	})
}

func (um *UploadManager) CancelSync(path string) {
	um.mu.Lock()
	defer um.mu.Unlock()

	// Cancel in-flight upload context so the Upload() call fails fast.
	if cancel, ok := um.cancelCtx[path]; ok {
		cancel()
		delete(um.cancelCtx, path)
	}

	if t, ok := um.timers[path]; ok {
		t.Stop()
		delete(um.timers, path)
	}
	delete(um.infos, path)
	delete(um.attempts, path)
	delete(um.retryCount, path)
	delete(um.inFlight, path)
}

func (um *UploadManager) IsPending(path string) bool {
	um.mu.Lock()
	defer um.mu.Unlock()
	_, ok := um.infos[path]
	return ok
}

func (um *UploadManager) uploadDone(path string, newFid string, encSize int64, err error, attempt *uploadAttempt) {
	um.mu.Lock()
	delete(um.inFlight, path)
	if _, stillDirty := um.infos[path]; stillDirty && err == nil && !um.shutdown.Load() {
		delay := um.writeBack
		if delay <= 0 {
			delay = 200 * time.Millisecond
		}
		um.timers[path] = um.newTimer(delay, func() {
			um.mu.Lock()
			if um.inFlight[path] {
				um.mu.Unlock()
				return
			}
			info, ok := um.infos[path]
			if !ok {
				um.mu.Unlock()
				return
			}
			att := um.attempts[path]
			delete(um.timers, path)
			um.inFlight[path] = true
			um.mu.Unlock()
			um.enqueueUpload(path, info, att)
		})
	}
	um.mu.Unlock()

	if err == nil && attempt != nil && attempt.Fid != "" {
		if um.coalescer != nil {
			um.coalescer.MarkUploadCommitted(attempt.Fid)
		}
	}

	if um.observer != nil {
		um.observer.OnUploadComplete(path, newFid, encSize, err, attempt)
	}
}

func (um *UploadManager) enqueueUpload(path string, info *uploadInfo, attempt *uploadAttempt) {
	if um.shutdown.Load() {
		um.mu.Lock()
		delete(um.infos, path)
		delete(um.inFlight, path)
		um.mu.Unlock()
		return
	}

	reader, err := info.StagingReader()
	if err != nil {
		logging.L.Warnf("upload: cannot open staging for %s: %v", path, err)
		um.mu.Lock()
		delete(um.infos, path)
		delete(um.inFlight, path)
		um.mu.Unlock()
		um.uploadDone(path, "", 0, err, attempt)
		return
	}
	defer reader.Close()

	// Derive per-upload context from rootCtx so CancelSync / Shutdown
	// can cancel in-flight uploads.
	uctx, uploadCancel := context.WithCancel(um.rootCtx)
	um.mu.Lock()
	um.cancelCtx[path] = uploadCancel
	um.mu.Unlock()
	defer func() {
		um.mu.Lock()
		delete(um.cancelCtx, path)
		um.mu.Unlock()
		uploadCancel()
	}()
	ctx := uctx

	// Notify coalescer that upload is starting (before the actual Upload call).
	if attempt != nil && attempt.Fid != "" {
		if um.coalescer != nil {
			um.coalescer.MarkUploadStarted(attempt.Fid)
		}
	}

	// Acquire concurrency slot; blocks when at capacity.
	um.sem <- struct{}{}
	current := atomic.AddInt32(&um.concurrentCount, 1)
	for {
		p := atomic.LoadInt32(&um.peakConcurrent)
		if current <= p || atomic.CompareAndSwapInt32(&um.peakConcurrent, p, current) {
			break
		}
	}
	defer func() {
		atomic.AddInt32(&um.concurrentCount, -1)
		<-um.sem
	}()

	entry, err := um.executor.Upload(ctx, info.ParentID, info.Name, info.Size, reader)
	newFid := entry.ID
	newSize := entry.Size
	if err == nil {
		um.mu.Lock()
		delete(um.infos, path)
		delete(um.attempts, path)
		delete(um.retryCount, path)
		um.mu.Unlock()
		um.uploadDone(path, newFid, newSize, nil, attempt)
		return
	}

	um.mu.Lock()
	delete(um.inFlight, path)
	count := um.retryCount[path] + 1
	um.retryCount[path] = count
	um.mu.Unlock()

	// If shutdown, skip retry: no new timers.
	if um.shutdown.Load() {
		um.mu.Lock()
		delete(um.infos, path)
		delete(um.attempts, path)
		delete(um.timers, path)
		delete(um.inFlight, path)
		um.mu.Unlock()
		count = um.maxRetries // fall through to observer notification
	} else if count < um.maxRetries {
		backoff := time.Duration(1<<uint(count)) * time.Second
		logging.L.Warnf("upload: %s failed (attempt %d/%d), retry in %v: %v",
			path, count, um.maxRetries, backoff, err)
		um.mu.Lock()
		if t, ok := um.timers[path]; ok {
			t.Stop()
		}
		um.timers[path] = um.newTimer(backoff, func() {
			um.mu.Lock()
			if um.inFlight[path] {
				um.mu.Unlock()
				return
			}
			info, ok := um.infos[path]
			attempt := um.attempts[path]
			delete(um.timers, path)
			um.mu.Unlock()
			if !ok {
				return
			}
			um.inFlight[path] = true
			um.enqueueUpload(path, info, attempt)
		})
		um.mu.Unlock()
	} else {
		logging.L.Errorf("upload: %s permanently failed after %d attempts: %v", path, count, err)
		um.mu.Lock()
		delete(um.infos, path)
		delete(um.attempts, path)
		delete(um.timers, path)
		delete(um.inFlight, path)
		um.mu.Unlock()
		um.uploadDone(path, "", 0, err, attempt)
	}
}

// FireAllTimers resets all pending timers to fire immediately (for testing).
func (um *UploadManager) FireAllTimers() {
	um.mu.Lock()
	defer um.mu.Unlock()
	for _, t := range um.timers {
		t.Reset(0)
	}
}

// Shutdown cancels all in-flight uploads, stops all pending timers, and waits
// for the semaphore to drain. After Shutdown returns, no upload goroutines
// are running. New MarkDirty calls still record state but do not schedule
// new uploads.
func (um *UploadManager) Shutdown() {
	um.shutdown.Store(true)

	um.mu.Lock()
	// Stop all pending timers.
	for p, t := range um.timers {
		t.Stop()
		delete(um.timers, p)
	}
	// Cancel all in-flight upload contexts so their Upload() calls fail fast.
	for p, cancel := range um.cancelCtx {
		cancel()
		delete(um.cancelCtx, p)
	}
	um.mu.Unlock()

	// Cancel the root context so any new per-upload derivations are pre-cancelled.
	um.rootCancel()

	// Wait for all in-flight upload goroutines to finish.
	for atomic.LoadInt32(&um.concurrentCount) > 0 {
		time.Sleep(10 * time.Millisecond)
	}
}
