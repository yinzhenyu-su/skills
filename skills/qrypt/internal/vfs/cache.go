package vfs

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/hashicorp/golang-lru/v2"
	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
)

// ReadBatchBlocks is the number of consecutive blocks fetched in a single request.
// Must match cipherdriver.ReadBatchBlocks (same value: 16 blocks ≈ 1MB).
const ReadBatchBlocks = 16

type FetchFunc func(ctx context.Context, fid string, startBlock uint64, count int) ([][]byte, error)

type dedupCall struct {
	ch   chan struct{}
	data []byte
	err  error
}

type ReadCache struct {
	memCache  *lru.Cache[string, []byte]
	diskCache *qrypt.FileBlockCache
	dedup     sync.Map
	fetcher   FetchFunc
}

func NewReadCache(memSizeMB int, diskCache *qrypt.FileBlockCache, fetcher FetchFunc) *ReadCache {
	maxEntries := (memSizeMB * 1024) / 64
	if maxEntries <= 0 {
		maxEntries = 256
	}
	memCache, _ := lru.New[string, []byte](maxEntries)

	return &ReadCache{
		memCache:  memCache,
		diskCache: diskCache,
		fetcher:   fetcher,
	}
}

func blockKey(fid string, blockIdx uint64) string {
	return fmt.Sprintf("%s_%d", fid, blockIdx)
}

func (cl *ReadCache) ReadBlock(ctx context.Context, fid string, blockIdx uint64) ([]byte, error) {
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

	// Align to batch boundary for efficient fetching.
	batchStart := (blockIdx / ReadBatchBlocks) * ReadBatchBlocks
	batchKey := blockKey(fid, batchStart)

	call := &dedupCall{ch: make(chan struct{})}
	actual, loaded := cl.dedup.LoadOrStore(batchKey, call)
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
		cl.dedup.Delete(batchKey)
	}()

	blocks, err := cl.fetcher(ctx, fid, batchStart, int(ReadBatchBlocks))
	if err != nil {
		call.err = err
		return nil, err
	}
	for i, data := range blocks {
		idx := batchStart + uint64(i)
		k := blockKey(fid, idx)
		cl.memCache.Add(k, data)
		if cl.diskCache != nil {
			_ = cl.diskCache.PutChunk(fid, int64(idx), data, false)
		}
	}

	if data, ok := cl.memCache.Get(key); ok {
		return data, nil
	}
	if cl.diskCache != nil {
		data, err := cl.diskCache.GetChunk(fid, int64(blockIdx))
		if err == nil && data != nil {
			return data, nil
		}
	}
	return nil, fmt.Errorf("block %s lost after batch fetch", key)
}

func (cl *ReadCache) NotifyRead(fid string, blockIdx uint64) {
	isFirstBlock := blockIdx%ReadBatchBlocks == 0
	if !isFirstBlock {
		return
	}
	nextBatch := ((blockIdx / ReadBatchBlocks) + 1) * ReadBatchBlocks
	go cl.prefetchBatch(fid, nextBatch)
}

func (cl *ReadCache) prefetchBatch(fid string, batchStart uint64) {
	key := blockKey(fid, batchStart)
	if cl.memCache.Contains(key) {
		return
	}

	call := &dedupCall{ch: make(chan struct{})}
	if _, loaded := cl.dedup.LoadOrStore(key, call); loaded {
		return
	}

	blocks, err := cl.fetcher(context.Background(), fid, batchStart, int(ReadBatchBlocks))
	if err != nil {
		close(call.ch)
		cl.dedup.Delete(key)
		return
	}
	for i, data := range blocks {
		idx := batchStart + uint64(i)
		k := blockKey(fid, idx)
		cl.memCache.Add(k, data)
		if cl.diskCache != nil {
			cl.diskCache.PutChunk(fid, int64(idx), data, false)
		}
	}
	close(call.ch)
	cl.dedup.Delete(key)
}

// InvalidateFid removes all cached entries for a file (both L1 and L2).
func (cl *ReadCache) InvalidateFid(fid string) {
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
