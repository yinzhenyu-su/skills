package daemon

import (
	"context"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter wraps x/time/rate.Limiter for file transfer rate limiting.
type RateLimiter struct {
	inner *rate.Limiter
}

// NewRateLimiter creates a rate limiter with the given bytes-per-second limit.
// If limit <= 0, the limiter is disabled (infinite).
func NewRateLimiter(bytesPerSec int64) *RateLimiter {
	if bytesPerSec <= 0 {
		return &RateLimiter{inner: nil}
	}
	r := rate.Limit(bytesPerSec)
	// Allow bursts up to 4x the rate (covers multipart upload chunks)
	return &RateLimiter{
		inner: rate.NewLimiter(r, int(bytesPerSec*4)),
	}
}

// Wait blocks until n bytes can be transmitted within the rate limit.
func (rl *RateLimiter) Wait(ctx context.Context, n int64) error {
	if rl.inner == nil {
		return nil
	}
	return rl.inner.WaitN(ctx, int(n))
}

// WaitFor for n bytes with a timeout.
func (rl *RateLimiter) WaitFor(ctx context.Context, n int64, timeout time.Duration) error {
	if rl.inner == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return rl.inner.WaitN(ctx, int(n))
}
