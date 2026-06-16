// Package drivercache provides a TTL-based cache for directory listings
// and negative lookups, extracted from the Quark driver so it can be reused
// by the cached driver middleware.
package drivercache

import (
	"sync"
	"time"

	"github.com/yinzhenyu/skills/qrypt/drivers"
)

// DirCache caches directory listings and negative lookups.
// It uses sync.Map internally and expires entries based on TTL.
type DirCache struct {
	dirCacheTTL time.Duration
	negCacheTTL time.Duration

	dirCache sync.Map
	negCache sync.Map
}

// dirCacheEntry holds a cached directory listing with its expiry time.
type dirCacheEntry struct {
	entries []drivers.Entry
	expiry  time.Time
}

// NewDirCache creates a DirCache with the given TTL for directory entries.
// If dirCacheTTL <= 0, a default of 60 seconds is used.
// The negative cache always uses a 60-second TTL.
func NewDirCache(dirCacheTTL time.Duration) *DirCache {
	if dirCacheTTL <= 0 {
		dirCacheTTL = 60 * time.Second
	}
	return &DirCache{
		dirCacheTTL: dirCacheTTL,
		negCacheTTL: 60 * time.Second,
	}
}

// GetDir retrieves a cached directory listing for a parent FID.
// Returns the entries and true on a cache hit, or nil and false on a miss/expiry.
func (c *DirCache) GetDir(parentFid string) ([]drivers.Entry, bool) {
	if val, ok := c.dirCache.Load(parentFid); ok {
		entry := val.(dirCacheEntry)
		if time.Now().Before(entry.expiry) {
			return entry.entries, true
		}
	}
	return nil, false
}

// SetDir caches a directory listing for a parent FID.
func (c *DirCache) SetDir(parentFid string, entries []drivers.Entry) {
	c.dirCache.Store(parentFid, dirCacheEntry{
		entries: entries,
		expiry:  time.Now().Add(c.dirCacheTTL),
	})
}

// RemoveDir removes a cached directory listing for a parent FID.
func (c *DirCache) RemoveDir(parentFid string) {
	c.dirCache.Delete(parentFid)
}

// GetNeg returns true if a negative lookup for (parentFid, name) is still valid.
func (c *DirCache) GetNeg(parentFid, name string) bool {
	key := parentFid + ":" + name
	if val, ok := c.negCache.Load(key); ok {
		expiry := val.(time.Time)
		if time.Now().Before(expiry) {
			return true
		}
	}
	return false
}

// SetNeg records a negative lookup for (parentFid, name).
func (c *DirCache) SetNeg(parentFid, name string) {
	key := parentFid + ":" + name
	c.negCache.Store(key, time.Now().Add(c.negCacheTTL))
}

// DeleteNeg removes a negative lookup for (parentFid, name).
func (c *DirCache) DeleteNeg(parentFid, name string) {
	key := parentFid + ":" + name
	c.negCache.Delete(key)
}
