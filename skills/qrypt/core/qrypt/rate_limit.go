package qrypt

import (
	"context"
	"time"

	"golang.org/x/time/rate"
)

type RateLimiter interface {
	Wait(ctx context.Context, n int64) error
	WaitFor(ctx context.Context, n int64, timeout time.Duration) error
}

type rateLimiter struct {
	inner *rate.Limiter
}

func NewRateLimiter(bytesPerSec int64) RateLimiter {
	if bytesPerSec <= 0 {
		return &rateLimiter{inner: nil}
	}
	r := rate.Limit(bytesPerSec)
	return &rateLimiter{
		inner: rate.NewLimiter(r, int(bytesPerSec*4)),
	}
}

func (rl *rateLimiter) Wait(ctx context.Context, n int64) error {
	if rl.inner == nil {
		return nil
	}
	return rl.inner.WaitN(ctx, int(n))
}

func (rl *rateLimiter) WaitFor(ctx context.Context, n int64, timeout time.Duration) error {
	if rl.inner == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return rl.inner.WaitN(ctx, int(n))
}
