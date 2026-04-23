package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/staging"
)

// CacheManager 协调磁盘存储和元数据数据库
type CacheManager struct {
	DB          *CacheDB
	cacheDir    string
	maxSize     int64
	staging     *staging.Store
}

// CacheDBPendingNode 定义待同步的节点（兼容 db.go 的 PendingNode）
type CacheDBPendingNode struct {
	Path            string
	Fid             string
	ParentFid       string
	Name            string
	LocalPath       string
	Size            int64
	IsFolder        bool
	Nonce           []byte
	BaseServerMtime int64
	BaseServerSize  int64
}

func (m *CacheManager) CacheDir() string {
	return m.cacheDir
}

func (m *CacheManager) StagingDir() string {
	return filepath.Join(m.cacheDir, "staging")
}

// Staging 返回 staging store 实例
func (m *CacheManager) Staging() *staging.Store {
	return m.staging
}

func (m *CacheManager) GetDB() interface{} {
	return m.DB
}

// NewCacheManager 创建缓存管理器
func NewCacheManager(cacheDir string, dbPath string, maxSize int64) (*CacheManager, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, err
	}

	// 如果 dbPath 不是绝对路径，拼接到 cacheDir 下
	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Join(cacheDir, dbPath)
	}

	db, err := NewCacheDB(dbPath)
	if err != nil {
		return nil, err
	}

	m := &CacheManager{
		DB:       db,
		cacheDir: cacheDir,
		maxSize:  maxSize,
	}

	// 初始化 staging store
	stagingDir := filepath.Join(cacheDir, "staging")
	store, err := staging.NewStore(stagingDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create staging store: %w", err)
	}
	// 设置元数据存储回调
	store.SetMetaStore(m)
	m.staging = store

	// 启动时清理孤立的 staging 文件
	m.cleanupOrphanedStagingFiles()

	return m, nil
}

func (m *CacheManager) cleanupOrphanedStagingFiles() {
	pendingNodes, err := m.DB.GetPendingNodes()
	if err != nil {
		fmt.Printf("cleanupOrphanedStagingFiles: failed to get pending nodes: %v\n", err)
		return
	}

	activeFids := make(map[string]bool)
	for _, n := range pendingNodes {
		if n.Fid != "" {
			activeFids[n.Fid] = true
		}
	}

	cleaned, err := m.staging.CleanupOrphanedStagingFiles(activeFids)
	if err != nil {
		fmt.Printf("cleanupOrphanedStagingFiles: failed: %v\n", err)
		return
	}

	if len(cleaned) > 0 {
		fmt.Printf("cleanupOrphanedStagingFiles: removed %d orphaned staging files\n", len(cleaned))
	}
}

// SaveStagingMeta 实现 MetaStore 接口
func (m *CacheManager) SaveStagingMeta(fid, localPath string, size int64) error {
	return m.DB.SaveStagingMeta(fid, localPath, size)
}

// UpdateStagingMeta 实现 MetaStore 接口
func (m *CacheManager) UpdateStagingMeta(fid string, size int64) error {
	return m.DB.UpdateStagingMeta(fid, size)
}

// RemoveStagingMeta 实现 MetaStore 接口
func (m *CacheManager) RemoveStagingMeta(fid string) error {
	return m.DB.RemoveStagingMeta(fid)
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

// HasChunk 检查本地缓存是否存在指定分块
func (m *CacheManager) HasChunk(fid string, chunkIndex int64) (bool, error) {
	_, found, err := m.DB.GetChunk(fid, chunkIndex)
	return found, err
}

// PutChunk 存储分块内容
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

	return m.DB.InsertChunk(fid, chunkIndex, path, int64(len(data)), isDirty)
}

// SavePendingNode 持久化未完成的文件节点
func (m *CacheManager) SavePendingNode(path, fid, parentFid, name, localPath string, size int64, isFolder bool, nonce []byte, baseMtime, baseSize int64) error {
	return m.DB.SavePendingNode(path, fid, parentFid, name, localPath, size, isFolder, nonce, baseMtime, baseSize)
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
		result = append(result, CacheDBPendingNode{
			Path:            n.Path,
			Fid:             n.Fid,
			ParentFid:       n.ParentFid,
			Name:            n.Name,
			LocalPath:       n.LocalPath,
			Size:            n.Size,
			IsFolder:        n.IsFolder,
			Nonce:           n.Nonce,
			BaseServerMtime: n.BaseServerMtime,
			BaseServerSize:  n.BaseServerSize,
		})
	}
	return result, nil
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

// RemoveChunksByFid 删除某个 fid 的本地缓存块
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

// EvictIfNeeded 检查并清理旧缓存
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
		os.Remove(c.Path)
		m.DB.DeleteChunk(c.Fid, c.Index)
	}

	return nil
}

// CleanupStagingMetas 清理过期的 staging 元数据
func (m *CacheManager) CleanupStagingMetas(abandonedMaxAge time.Duration) error {
	pendingNodes, err := m.DB.GetPendingNodes()
	if err != nil {
		return err
	}

	activeFids := make(map[string]bool)
	for _, n := range pendingNodes {
		if n.Fid != "" {
			activeFids[n.Fid] = true
		}
	}

	allMetas, err := m.DB.GetStagingMetasByStatus("active")
	if err != nil {
		return err
	}
	orphanFids := []string{}
	for _, meta := range allMetas {
		if !activeFids[meta.Fid] {
			orphanFids = append(orphanFids, meta.Fid)
		}
	}

	return m.DB.CleanupStagingMetas(abandonedMaxAge, orphanFids)
}

func (m *CacheManager) Maintenance() error {
	if err := m.CleanupStagingMetas(24 * time.Hour); err != nil {
		fmt.Printf("CleanupStagingMetas failed: %v\n", err)
	}
	return m.DB.Maintenance()
}

func (m *CacheManager) MaintenanceStart() {
	go func() {
		if err := m.Maintenance(); err != nil {
			fmt.Printf("Background maintenance failed: %v\n", err)
		}
	}()
}

func (m *CacheManager) Close() error {
	return m.DB.Close()
}

func (m *CacheManager) BatchDeleteNodeState(fids []string, paths []string) error {
	return m.DB.BatchDeleteNodeState(fids, paths)
}

func (m *CacheManager) BatchMarkOpsDone(fids []string, paths []string) error {
	return m.DB.BatchMarkOpsDone(fids, paths)
}
