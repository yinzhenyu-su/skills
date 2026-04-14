package cache

import (
	"fmt"
	"os"
	"path/filepath"
)

// CacheManager 协调磁盘存储和元数据数据库
type CacheManager struct {
	DB       *CacheDB
	cacheDir string
	maxSize  int64
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
		DB:       db,
		cacheDir: cacheDir,
		maxSize:  maxSize,
	}, nil
}

// GetChunk 读取分块内容
func (m *CacheManager) GetChunk(fid string, chunkIndex int64) ([]byte, error) {
	path, found, err := m.DB.GetChunk(fid, chunkIndex)
	if err != nil || !found {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// 异步更新访问时间 (LRU)
	go m.DB.UpdateAccessTime(fid, chunkIndex)
	return data, nil
}

// PutChunk 存储分块内容 (解密后的数据或待上传的数据)
func (m *CacheManager) PutChunk(fid string, chunkIndex int64, data []byte, isDirty bool) error {
	suffix := ".dec.chunk"
	if isDirty {
		suffix = ".dirty.chunk"
	}
	fileName := fmt.Sprintf("%s_%d%s", fid, chunkIndex, suffix)
	path := filepath.Join(m.cacheDir, fileName)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}

	// 存入数据库
	return m.DB.InsertChunk(fid, chunkIndex, path, int64(len(data)), isDirty)
}

// SavePendingNode 持久化未完成的文件节点
func (m *CacheManager) SavePendingNode(path, fid, name string, size int64, isFolder bool, nonce []byte) error {
	return m.DB.SavePendingNode(path, fid, name, size, isFolder, nonce)
}

// RemovePendingNode 移除已完成的文件节点
func (m *CacheManager) RemovePendingNode(path string) error {
	return m.DB.RemovePendingNode(path)
}

// RemovePendingNodesByPrefix 按路径前缀移除待同步节点
func (m *CacheManager) RemovePendingNodesByPrefix(prefix string) error {
	return m.DB.RemovePendingNodesByPrefix(prefix)
}

// RemovePendingNodesByFid 按 fid 移除待同步节点
func (m *CacheManager) RemovePendingNodesByFid(fid string) error {
	return m.DB.RemovePendingNodesByFid(fid)
}

// GetPendingNodes 获取所有待同步的节点
func (m *CacheManager) GetPendingNodes() ([]CacheDBPendingNode, error) {
	nodes, err := m.DB.GetPendingNodes()
	if err != nil {
		return nil, err
	}
	var result []CacheDBPendingNode
	for _, n := range nodes {
		result = append(result, CacheDBPendingNode(n))
	}
	return result, nil
}

type CacheDBPendingNode struct {
	Path     string
	Fid      string
	Name     string
	Size     int64
	IsFolder bool
	Nonce    []byte
}

// GetDirtyChunks 获取文件的所有脏分块索引
func (m *CacheManager) GetDirtyChunks(fid string) ([]int64, error) {
	return m.DB.GetDirtyChunks(fid)
}

// GetCachedName 获取解密文件名缓存
func (m *CacheManager) GetCachedName(fid, encryptedName string) (string, bool, error) {
	return m.DB.GetCachedName(fid, encryptedName)
}

// SaveCachedName 保存解密文件名缓存
func (m *CacheManager) SaveCachedName(fid, encryptedName, decryptedName string) error {
	return m.DB.SaveCachedName(fid, encryptedName, decryptedName)
}

// RemoveChunksByFid 删除某个 fid 的本地缓存块（文件与元数据）
func (m *CacheManager) RemoveChunksByFid(fid string) error {
	paths, err := m.DB.GetChunkPathsByFid(fid)
	if err != nil {
		return err
	}
	for _, p := range paths {
		_ = os.Remove(p)
	}
	return m.DB.DeleteChunksByFid(fid)
}

// EvictIfNeeded 检查并清理旧缓存，如果超出最大值
func (m *CacheManager) EvictIfNeeded(lowWatermark int64) error {
	total, err := m.DB.GetTotalSize()
	if err != nil {
		return err
	}

	if total <= m.maxSize {
		return nil
	}

	targetSize := total - lowWatermark
	chunks, err := m.DB.GetOldestChunks(targetSize)
	if err != nil {
		return err
	}

	for _, c := range chunks {
		os.Remove(c.Path) // 忽略删除错误
		m.DB.DeleteChunk(c.Fid, c.Index)
	}

	return nil
}

// Close 关闭缓存管理器
func (m *CacheManager) Close() error {
	return m.DB.Close()
}
