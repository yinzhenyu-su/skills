package qrypt

import (
	"context"
	"sync"
)

type Orchestrator interface {
	Submit(job func(ctx context.Context) error) bool
	Shutdown()
}

type orchestrator struct {
	queue        chan func(ctx context.Context) error
	wg           sync.WaitGroup
	cancel       context.CancelFunc
	tokenBucket  RateLimiter
	progressHub  ProgressHub
	shutdownOnce sync.Once
}

func NewOrchestrator(nWorkers int, rl RateLimiter, ph ProgressHub) Orchestrator {
	ctx, cancel := context.WithCancel(context.Background())
	o := &orchestrator{
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

func (o *orchestrator) worker(ctx context.Context) {
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
			_ = fn(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (o *orchestrator) Submit(fn func(ctx context.Context) error) bool {
	select {
	case o.queue <- fn:
		return true
	default:
		return false
	}
}

func (o *orchestrator) Shutdown() {
	o.shutdownOnce.Do(func() {
		o.cancel()
		close(o.queue)
		o.wg.Wait()
	})
}
