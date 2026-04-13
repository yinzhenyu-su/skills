package cache

import (
	"fmt"
	"os"
	"path/filepath"
)

// CacheManager 协调磁盘存储和元数据数据库
type CacheManager struct {
	db        *CacheDB
	cacheDir  string
	maxSize   int64
}

// NewCacheManager 创建缓存管理器
func NewCacheManager(cacheDir string, dbPath string, maxSize int64) (*CacheManager, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, err
	}

	db, err := NewCacheDB(dbPath)
	if err != nil {
		return nil, err
	}

	return &CacheManager{
		db:       db,
		cacheDir: cacheDir,
		maxSize:  maxSize,
	}, nil
}

// GetChunk 读取分块内容
func (m *CacheManager) GetChunk(fid string, chunkIndex int64) ([]byte, error) {
	path, found, err := m.db.GetChunk(fid, chunkIndex)
	if err != nil || !found {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// 异步更新访问时间 (LRU)
	go m.db.UpdateAccessTime(fid, chunkIndex)
	return data, nil
}

// PutChunk 存储分块内容 (解密后的数据)
func (m *CacheManager) PutChunk(fid string, chunkIndex int64, data []byte) error {
	fileName := fmt.Sprintf("%s_%d.dec.chunk", fid, chunkIndex)
	path := filepath.Join(m.cacheDir, fileName)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}

	// 存入数据库，标记为非 dirty（即已解密）
	return m.db.InsertChunk(fid, chunkIndex, path, int64(len(data)), false)
}

// EvictIfNeeded 检查并清理旧缓存，如果超出最大值
func (m *CacheManager) EvictIfNeeded(lowWatermark int64) error {
	total, err := m.db.GetTotalSize()
	if err != nil {
		return err
	}

	if total <= m.maxSize {
		return nil
	}

	targetSize := total - lowWatermark
	chunks, err := m.db.GetOldestChunks(targetSize)
	if err != nil {
		return err
	}

	for _, c := range chunks {
		os.Remove(c.Path) // 忽略删除错误
		m.db.DeleteChunk(c.Fid, c.Index)
	}

	return nil
}

// Close 关闭缓存管理器
func (m *CacheManager) Close() error {
	return m.db.Close()
}
