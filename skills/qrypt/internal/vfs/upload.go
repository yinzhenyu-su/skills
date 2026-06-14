package vfs

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
)

type uploadInfo struct {
	ParentID      string
	Name          string
	Size          int64
	StagingReader func() (io.ReadCloser, error)
}

// timerFactory is a pluggable timer constructor for test injection.
type timerFactory func(d time.Duration, f func()) *time.Timer

// Uploader abstracts the upload backend for testability.
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
}

func NewUploadManager(executor Uploader, writeBack time.Duration, observer UploadObserver) *UploadManager {
	return &UploadManager{
		executor:   executor,
		observer:   observer,
		writeBack:  writeBack,
		maxRetries: 5,
		newTimer:   time.AfterFunc,
		timers:     make(map[string]*time.Timer),
		retryCount: make(map[string]int),
		infos:      make(map[string]*uploadInfo),
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

func (um *UploadManager) MarkDirty(path string, parentID, name string, size int64, stagingReader func() (io.ReadCloser, error)) {
	um.mu.Lock()
	defer um.mu.Unlock()

	um.infos[path] = &uploadInfo{
		ParentID:      parentID,
		Name:          name,
		Size:          size,
		StagingReader: stagingReader,
	}

	if t, ok := um.timers[path]; ok {
		t.Stop()
	}

	delay := um.writeBack
	if delay <= 0 {
		delay = 200 * time.Millisecond
	}
	um.timers[path] = um.newTimer(delay, func() {
		um.mu.Lock()
		info, ok := um.infos[path]
		if !ok {
			um.mu.Unlock()
			return
		}
		delete(um.timers, path)
		um.mu.Unlock()
		um.enqueueUpload(path, info)
	})
}

func (um *UploadManager) CancelSync(path string) {
	um.mu.Lock()
	defer um.mu.Unlock()
	if t, ok := um.timers[path]; ok {
		t.Stop()
		delete(um.timers, path)
	}
	delete(um.infos, path)
	delete(um.retryCount, path)
}

func (um *UploadManager) IsPending(path string) bool {
	um.mu.Lock()
	defer um.mu.Unlock()
	_, ok := um.infos[path]
	return ok
}

func (um *UploadManager) enqueueUpload(path string, info *uploadInfo) {
	reader, err := info.StagingReader()
	if err != nil {
		logging.L.Warnf("upload: cannot open staging for %s: %v", path, err)
		um.mu.Lock()
		delete(um.infos, path)
		um.mu.Unlock()
		if um.observer != nil {
			um.observer.OnUploadComplete(path, err)
		}
		return
	}
	defer reader.Close()

	ctx := context.Background()
	_, err = um.executor.Upload(ctx, info.ParentID, info.Name, info.Size, reader)
	if err == nil {
		um.mu.Lock()
		delete(um.infos, path)
		delete(um.retryCount, path)
		um.mu.Unlock()
		if um.observer != nil {
			um.observer.OnUploadComplete(path, nil)
		}
		return
	}

	um.mu.Lock()
	count := um.retryCount[path] + 1
	um.retryCount[path] = count
	um.mu.Unlock()

	if count < um.maxRetries {
		backoff := time.Duration(1<<uint(count)) * time.Second
		logging.L.Warnf("upload: %s failed (attempt %d/%d), retry in %v: %v",
			path, count, um.maxRetries, backoff, err)
		um.mu.Lock()
		if t, ok := um.timers[path]; ok {
			t.Stop()
		}
		um.timers[path] = um.newTimer(backoff, func() {
			um.mu.Lock()
			info, ok := um.infos[path]
			delete(um.timers, path)
			um.mu.Unlock()
			if !ok {
				return
			}
			um.enqueueUpload(path, info)
		})
		um.mu.Unlock()
	} else {
		logging.L.Errorf("upload: %s permanently failed after %d attempts: %v", path, count, err)
		um.mu.Lock()
		delete(um.infos, path)
		delete(um.timers, path)
		um.mu.Unlock()
		if um.observer != nil {
			um.observer.OnUploadComplete(path, err)
		}
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
