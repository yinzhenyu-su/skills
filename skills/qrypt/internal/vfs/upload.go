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
	OldFid        string
	StagingReader func() (io.ReadCloser, error)
}

// timerFactory is a pluggable timer constructor for test injection.
type timerFactory func(d time.Duration, f func()) *time.Timer

type Uploader interface {
	Upload(ctx context.Context, parentID, name string, size int64, body io.Reader) (drivers.Entry, error)
}

type Remover interface {
	Remove(ctx context.Context, entry drivers.Entry) error
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
	inFlight   map[string]bool // paths currently being uploaded
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
	inFlight:   make(map[string]bool),
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

func (um *UploadManager) MarkDirty(path string, parentID, name string, size int64, oldFid string, stagingReader func() (io.ReadCloser, error)) {
	um.mu.Lock()
	defer um.mu.Unlock()

	um.infos[path] = &uploadInfo{
		ParentID:      parentID,
		Name:          name,
		Size:          size,
		OldFid:        oldFid,
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
		delete(um.timers, path)
		um.inFlight[path] = true
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
	delete(um.inFlight, path)
}

func (um *UploadManager) IsPending(path string) bool {
	um.mu.Lock()
	defer um.mu.Unlock()
	_, ok := um.infos[path]
	return ok
}

func (um *UploadManager) uploadDone(path string, newFid string, err error) {
	um.mu.Lock()
	delete(um.inFlight, path)
	if _, stillDirty := um.infos[path]; stillDirty && err == nil {
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
			delete(um.timers, path)
			um.inFlight[path] = true
			um.mu.Unlock()
			um.enqueueUpload(path, info)
		})
	}
	um.mu.Unlock()
	if um.observer != nil {
		um.observer.OnUploadComplete(path, newFid, err)
	}
}

func (um *UploadManager) enqueueUpload(path string, info *uploadInfo) {
	reader, err := info.StagingReader()
	if err != nil {
		logging.L.Warnf("upload: cannot open staging for %s: %v", path, err)
		um.mu.Lock()
		delete(um.infos, path)
		delete(um.inFlight, path)
		um.mu.Unlock()
		um.uploadDone(path, "", err)
		return
	}
	defer reader.Close()

	ctx := context.Background()

	if info.OldFid != "" {
		if rem, ok := um.executor.(Remover); ok {
			_ = rem.Remove(ctx, drivers.Entry{ID: info.OldFid})
		}
	}

	entry, err := um.executor.Upload(ctx, info.ParentID, info.Name, info.Size, reader)
	newFid := entry.ID
	if err == nil {
		um.mu.Lock()
		delete(um.infos, path)
		delete(um.retryCount, path)
		um.mu.Unlock()
		um.uploadDone(path, newFid, nil)
		return
	}

	um.mu.Lock()
	delete(um.inFlight, path)
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
			if um.inFlight[path] {
				um.mu.Unlock()
				return
			}
			info, ok := um.infos[path]
			delete(um.timers, path)
			um.mu.Unlock()
			if !ok {
				return
			}
			um.inFlight[path] = true
			um.enqueueUpload(path, info)
		})
		um.mu.Unlock()
	} else {
		logging.L.Errorf("upload: %s permanently failed after %d attempts: %v", path, count, err)
		um.mu.Lock()
		delete(um.infos, path)
		delete(um.timers, path)
		delete(um.inFlight, path)
		um.mu.Unlock()
		um.uploadDone(path, "", err)
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
