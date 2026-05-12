package quark

import (
	"sync"
	"time"
)

type CacheService struct {
	DirCacheTTL  time.Duration
	NegCacheTTL  time.Duration
	urlCache     sync.Map
	dirCache     sync.Map
	negCache     sync.Map
}

type cachedURL struct {
	url    string
	expiry time.Time
}

type dirCacheEntry struct {
	files  []File
	expiry time.Time
}

func NewCacheService() *CacheService {
	return &CacheService{
		DirCacheTTL: 60 * time.Second,
		NegCacheTTL: 60 * time.Second,
	}
}

func (c *CacheService) GetURL(fid string) (string, bool) {
	if val, ok := c.urlCache.Load(fid); ok {
		cu := val.(cachedURL)
		if time.Now().Before(cu.expiry) {
			return cu.url, true
		}
	}
	return "", false
}

func (c *CacheService) SetURL(fid, url string) {
	c.urlCache.Store(fid, cachedURL{
		url:    url,
		expiry: time.Now().Add(10 * time.Minute),
	})
}

func (c *CacheService) InvalidateURL(fid string) {
	c.urlCache.Delete(fid)
}

func (c *CacheService) GetDir(parentFid string) ([]File, bool) {
	if val, ok := c.dirCache.Load(parentFid); ok {
		entry := val.(dirCacheEntry)
		if time.Now().Before(entry.expiry) {
			return entry.files, true
		}
	}
	return nil, false
}

func (c *CacheService) SetDir(parentFid string, files []File) {
	c.dirCache.Store(parentFid, dirCacheEntry{
		files:  files,
		expiry: time.Now().Add(c.DirCacheTTL),
	})
}

func (c *CacheService) RemoveDir(parentFid string) {
	c.dirCache.Delete(parentFid)
}

func (c *CacheService) GetNeg(parentFid, name string) bool {
	key := parentFid + ":" + name
	if val, ok := c.negCache.Load(key); ok {
		expiry := val.(time.Time)
		if time.Now().Before(expiry) {
			return true
		}
	}
	return false
}

func (c *CacheService) SetNeg(parentFid, name string) {
	key := parentFid + ":" + name
	c.negCache.Store(key, time.Now().Add(c.NegCacheTTL))
}

func (c *CacheService) DeleteNeg(parentFid, name string) {
	key := parentFid + ":" + name
	c.negCache.Delete(key)
}
