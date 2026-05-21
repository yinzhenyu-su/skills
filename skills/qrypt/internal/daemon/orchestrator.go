package daemon

import (
	"context"
	"sync"

	"github.com/yinzhenyu/skills/qrypt/internal/log"
)

// Orchestrator provides a shared worker pool for all uploads (VFS flush + CLI push).
// It implements fs.UploadQueue by accepting closures of type func(ctx) error.
type Orchestrator struct {
	queue       chan func(ctx context.Context) error
	wg          sync.WaitGroup
	cancel      context.CancelFunc
	tokenBucket *RateLimiter
	progressHub *ProgressHub
	shutdownOnce sync.Once
}

// NewOrchestrator creates an orchestrator with nWorkers goroutines.
func NewOrchestrator(nWorkers int, rl *RateLimiter, ph *ProgressHub) *Orchestrator {
	ctx, cancel := context.WithCancel(context.Background())
	o := &Orchestrator{
		queue:       make(chan func(ctx context.Context) error, 2000),
		cancel:      cancel,
		tokenBucket: rl,
		progressHub: ph,
	}
	for i := 0; i < nWorkers; i++ {
		o.wg.Add(1)
		go o.worker(ctx)
	}
	return o
}

func (o *Orchestrator) worker(ctx context.Context) {
	defer o.wg.Done()
	for {
		select {
		case fn, ok := <-o.queue:
			if !ok {
				return
			}
			if o.tokenBucket != nil {
				_ = o.tokenBucket.Wait(ctx, 0)
			}
			if err := fn(ctx); err != nil {
				log.L.Errorf("Orchestrator: upload failed: %v\n", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

// Submit enqueues a closure-based upload job. Implements fs.UploadQueue.
func (o *Orchestrator) Submit(fn func(ctx context.Context) error) bool {
	select {
	case o.queue <- fn:
		return true
	default:
		return false
	}
}

// Shutdown waits for all workers to finish. Idempotent — safe to call multiple times.
func (o *Orchestrator) Shutdown() {
	o.shutdownOnce.Do(func() {
		o.cancel()
		close(o.queue)
		o.wg.Wait()
	})
}
