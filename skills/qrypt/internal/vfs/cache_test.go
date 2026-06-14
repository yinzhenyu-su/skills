package vfs

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
)

// newTestCacheLayer creates a CacheLayer backed by a temp-dir disk cache,
// with a controlled fetcher that tracks calls via callCount.
func newTestCacheLayer(t *testing.T, callCount *atomic.Int64, data []byte, fetchErr error) (*CacheLayer, *qrypt.CacheManager) {
	t.Helper()

	fetcher := func(ctx context.Context, fid string, startBlock uint64, count int) ([][]byte, error) {
		callCount.Add(1)
		if data == nil {
			return nil, fetchErr
		}
		blocks := make([][]byte, count)
		for i := 0; i < count; i++ {
			blocks[i] = data
		}
		return blocks, fetchErr
	}

	tmpDir := t.TempDir()
	diskCache, err := qrypt.NewCacheManager(tmpDir, 500*1024*1024)
	require.NoError(t, err)
	t.Cleanup(func() { diskCache.Close() })

	cl := NewCacheLayer(64, diskCache, fetcher)
	return cl, diskCache
}

// TestCacheLayer_L1Hit verifies the second ReadBlock hits the in-memory LRU
// and does not call the fetcher again.
func TestCacheLayer_L1Hit(t *testing.T) {
	t.Parallel()

	data := []byte("block-data")
	var callCount atomic.Int64
	cl, _ := newTestCacheLayer(t, &callCount, data, nil)

	result, err := cl.ReadBlock(context.Background(), "fid", 0)
	require.NoError(t, err)
	assert.Equal(t, data, result)
	assert.Equal(t, int64(1), callCount.Load())

	// Second call: should hit L1 (memCache), no fetcher call.
	result, err = cl.ReadBlock(context.Background(), "fid", 0)
	require.NoError(t, err)
	assert.Equal(t, data, result)
	assert.Equal(t, int64(1), callCount.Load(), "fetcher should not be called on L1 hit")
}

// TestCacheLayer_L2Hit verifies a fresh CacheLayer (empty L1) can still serve
// data from the shared disk cache without calling the fetcher.
func TestCacheLayer_L2Hit(t *testing.T) {
	t.Parallel()

	data := []byte("block-data")
	var callCount atomic.Int64

	fetcher := func(ctx context.Context, fid string, startBlock uint64, count int) ([][]byte, error) {
		callCount.Add(1)
		blocks := make([][]byte, count)
		for i := 0; i < count; i++ {
			blocks[i] = data
		}
		return blocks, nil
	}

	tmpDir := t.TempDir()
	diskCache, err := qrypt.NewCacheManager(tmpDir, 500*1024*1024)
	require.NoError(t, err)
	t.Cleanup(func() { diskCache.Close() })

	// First CacheLayer: populate L1 + disk.
	cl1 := NewCacheLayer(64, diskCache, fetcher)
	result, err := cl1.ReadBlock(context.Background(), "fid", 0)
	require.NoError(t, err)
	assert.Equal(t, data, result)

	// Second CacheLayer: fresh L1, same disk → should hit L2.
	cl2 := NewCacheLayer(64, diskCache, fetcher)
	result, err = cl2.ReadBlock(context.Background(), "fid", 0)
	require.NoError(t, err)
	assert.Equal(t, data, result)

	assert.Equal(t, int64(1), callCount.Load(), "fetcher should be called only once (L2 hit)")
}

// TestCacheLayer_FetcherCalled verifies the fetcher is invoked for each
// unique block and returns the correct data.
func TestCacheLayer_FetcherCalled(t *testing.T) {
	t.Parallel()

	data0 := []byte("block-zero")
	data1 := []byte("block-one")
	var fetcherCalls atomic.Int64

	fetcher := func(ctx context.Context, fid string, startBlock uint64, count int) ([][]byte, error) {
		fetcherCalls.Add(1)
		blocks := make([][]byte, count)
		for i := 0; i < count; i++ {
			idx := startBlock + uint64(i)
			if idx == 0 {
				blocks[i] = data0
			} else {
				blocks[i] = data1
			}
		}
		return blocks, nil
	}

	tmpDir := t.TempDir()
	diskCache, err := qrypt.NewCacheManager(tmpDir, 500*1024*1024)
	require.NoError(t, err)
	t.Cleanup(func() { diskCache.Close() })

	cl := NewCacheLayer(64, diskCache, fetcher)

	result, err := cl.ReadBlock(context.Background(), "fid", 0)
	require.NoError(t, err)
	assert.Equal(t, data0, result)

	// Block 16 is in a different batch (batch[16-31]) than block 0 (batch[0-15]).
	result, err = cl.ReadBlock(context.Background(), "fid", 16)
	require.NoError(t, err)
	assert.Equal(t, data1, result)

	assert.Equal(t, int64(2), fetcherCalls.Load(), "fetcher should be called for each unique batch")
}

// TestCacheLayer_ConcurrentDedup verifies that concurrent reads of the same
// (fid, blockIdx) coalesce into a single fetch call.
func TestCacheLayer_ConcurrentDedup(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int64
	startCh := make(chan struct{})
	data := []byte("dedup-data")

	fetcher := func(ctx context.Context, fid string, startBlock uint64, count int) ([][]byte, error) {
		callCount.Add(1)
		<-startCh // block until test releases
		blocks := make([][]byte, count)
		for i := 0; i < count; i++ {
			blocks[i] = data
		}
		return blocks, nil
	}

	cl, _ := newTestCacheLayer(t, &callCount, nil, nil)
	// Override with the blocking fetcher.
	tmpDir := t.TempDir()
	diskCache, err := qrypt.NewCacheManager(tmpDir, 500*1024*1024)
	require.NoError(t, err)
	t.Cleanup(func() { diskCache.Close() })
	cl = NewCacheLayer(64, diskCache, fetcher)

	const numGoroutines = 3
	var wg sync.WaitGroup
	results := make([][]byte, numGoroutines)
	errs := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = cl.ReadBlock(context.Background(), "fid", 0)
		}(i)
	}

	// Let all goroutines reach the dedup barrier.
	time.Sleep(100 * time.Millisecond)
	close(startCh) // release fetcher
	wg.Wait()

	for i := 0; i < numGoroutines; i++ {
		require.NoError(t, errs[i], "goroutine %d", i)
		assert.Equal(t, data, results[i], "goroutine %d", i)
	}

	assert.Equal(t, int64(1), callCount.Load(), "fetcher should be called exactly once for 3 concurrent reads")
}

// TestCacheLayer_FetcherError verifies errors are propagated to callers and
// are NOT cached (subsequent calls retry the fetcher).
func TestCacheLayer_FetcherError(t *testing.T) {
	t.Parallel()

	testErr := errors.New("fetch failed")
	var callCount atomic.Int64
	cl, _ := newTestCacheLayer(t, &callCount, nil, testErr)

	_, err := cl.ReadBlock(context.Background(), "fid", 0)
	assert.ErrorIs(t, err, testErr)
	assert.Equal(t, int64(1), callCount.Load())

	// Second call: dedup entry is cleaned up, so fetcher is invoked again.
	_, err = cl.ReadBlock(context.Background(), "fid", 0)
	assert.ErrorIs(t, err, testErr)
	assert.Equal(t, int64(2), callCount.Load(), "error should not be cached; fetcher called again")
}

// TestCacheLayer_InvalidateFid verifies InvalidateFid clears both L1 and L2
// entries for the target fid without affecting other fids.
func TestCacheLayer_InvalidateFid(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int64
	fetcher := func(ctx context.Context, fid string, startBlock uint64, count int) ([][]byte, error) {
		callCount.Add(1)
		var blockData []byte
		if fid == "f1" {
			blockData = []byte("data-f1")
		} else {
			blockData = []byte("data-f2")
		}
		blocks := make([][]byte, count)
		for i := 0; i < count; i++ {
			blocks[i] = blockData
		}
		return blocks, nil
	}

	tmpDir := t.TempDir()
	diskCache, err := qrypt.NewCacheManager(tmpDir, 500*1024*1024)
	require.NoError(t, err)
	t.Cleanup(func() { diskCache.Close() })

	cl := NewCacheLayer(64, diskCache, fetcher)

	// Populate caches for both fids.
	_, err = cl.ReadBlock(context.Background(), "f1", 0)
	require.NoError(t, err)
	_, err = cl.ReadBlock(context.Background(), "f2", 0)
	require.NoError(t, err)
	assert.Equal(t, int64(2), callCount.Load())

	// Invalidate f1.
	cl.InvalidateFid("f1")

	// After invalidation, f1 block 0 should trigger a fetcher call.
	_, err = cl.ReadBlock(context.Background(), "f1", 0)
	require.NoError(t, err)
	assert.Equal(t, int64(3), callCount.Load(), "fetcher should be called after invalidation of f1")

	// f2 should still be cached (no additional fetcher call).
	_, err = cl.ReadBlock(context.Background(), "f2", 0)
	require.NoError(t, err)
	assert.Equal(t, int64(3), callCount.Load(), "fetcher should NOT be called for f2 (still cached)")
}
