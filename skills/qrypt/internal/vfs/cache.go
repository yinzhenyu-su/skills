package vfs

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/hashicorp/golang-lru/v2"
	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
)

// FetchFunc retrieves a decrypted block from the remote via CipherDriver.
type FetchFunc func(ctx context.Context, fid string, blockIdx uint64) ([]byte, error)

type dedupCall struct {
	ch   chan struct{}
	data []byte
	err  error
}

// CacheLayer provides L1 (mem) and L2 (disk) caching for decrypted blocks,
// with concurrent fetch deduplication.
type CacheLayer struct {
	memCache  *lru.Cache[string, []byte]
	diskCache *qrypt.CacheManager
	dedup     sync.Map // key → *dedupCall
	fetcher   FetchFunc
}

// NewCacheLayer creates a CacheLayer with the given memory cache size in MB,
// disk cache, and fetch callback.
func NewCacheLayer(memSizeMB int, diskCache *qrypt.CacheManager, fetcher FetchFunc) *CacheLayer {
	maxEntries := (memSizeMB * 1024) / 64
	if maxEntries <= 0 {
		maxEntries = 256
	}
	memCache, _ := lru.New[string, []byte](maxEntries)

	return &CacheLayer{
		memCache:  memCache,
		diskCache: diskCache,
		fetcher:   fetcher,
	}
}

func blockKey(fid string, blockIdx uint64) string {
	return fmt.Sprintf("%s_%d", fid, blockIdx)
}

// ReadBlock returns plaintext block data, checking L1 → L2 → fetcher.
func (cl *CacheLayer) ReadBlock(ctx context.Context, fid string, blockIdx uint64) ([]byte, error) {
	key := blockKey(fid, blockIdx)

	if data, ok := cl.memCache.Get(key); ok {
		return data, nil
	}

	if cl.diskCache != nil {
		data, err := cl.diskCache.GetChunk(fid, int64(blockIdx))
		if err == nil && data != nil {
			cl.memCache.Add(key, data)
			return data, nil
		}
	}

	call := &dedupCall{ch: make(chan struct{})}
	actual, loaded := cl.dedup.LoadOrStore(key, call)
	if loaded {
		entry := actual.(*dedupCall)
		select {
		case <-entry.ch:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		if entry.err != nil {
			return nil, entry.err
		}
		if entry.data != nil {
			return entry.data, nil
		}
		if data, ok := cl.memCache.Get(key); ok {
			return data, nil
		}
		if cl.diskCache != nil {
			data, err := cl.diskCache.GetChunk(fid, int64(blockIdx))
			if err == nil && data != nil {
				cl.memCache.Add(key, data)
				return data, nil
			}
		}
		return nil, fmt.Errorf("block %s not available after concurrent fetch", key)
	}

	defer func() {
		close(call.ch)
		cl.dedup.Delete(key)
	}()

	data, err := cl.fetcher(ctx, fid, blockIdx)
	if err != nil {
		call.err = err
		return nil, err
	}
	call.data = data

	if cl.diskCache != nil {
		cl.diskCache.PutChunk(fid, int64(blockIdx), data, false)
	}

	cl.memCache.Add(key, data)

	return data, nil
}

// InvalidateFid removes all cached entries for a file (both L1 and L2).
func (cl *CacheLayer) InvalidateFid(fid string) {
	if cl.diskCache != nil {
		cl.diskCache.RemoveChunksByFid(fid)
	}

	prefix := fid + "_"
	for _, key := range cl.memCache.Keys() {
		if strings.HasPrefix(key, prefix) {
			cl.memCache.Remove(key)
		}
	}
}
