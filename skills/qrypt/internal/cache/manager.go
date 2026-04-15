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
	Path      string
	Fid       string
	ParentFid string
	Name      string
	LocalPath string
	Size      int64
	IsFolder  bool
	Nonce     []byte
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

// NewCacheManager 创建缓存管理器
// 注意：Maintenance() 需要在业务低峰期手动调用，详见 Maintenance() 文档
func NewCacheManager(cacheDir string, dbPath string, maxSize int64) (*CacheManager, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, err
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

// cleanupOrphanedStagingFiles 清理孤立的 staging 文件
// 孤立文件：staging 目录中存在但没有对应 pending node 的文件
func (m *CacheManager) cleanupOrphanedStagingFiles() {
	// 获取所有 pending nodes 关联的 fid
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

	// 清理孤立文件
	cleaned, err := m.staging.CleanupOrphanedStagingFiles(activeFids)
	if err != nil {
		fmt.Printf("cleanupOrphanedStagingFiles: failed: %v\n", err)
		return
	}

	if len(cleaned) > 0 {
		fmt.Printf("cleanupOrphanedStagingFiles: removed %d orphaned staging files\n", len(cleaned))
	}
}

// --- MetaStore 接口实现 (供 staging.Store 回调) ---

// SaveStagingMeta 保存 staging 文件元数据
func (m *CacheManager) SaveStagingMeta(fid, localPath string, size int64) error {
	return m.DB.SaveStagingMeta(fid, localPath, size)
}

// UpdateStagingMeta 更新 staging 文件元数据
func (m *CacheManager) UpdateStagingMeta(fid string, size int64) error {
	return m.DB.UpdateStagingMeta(fid, size)
}

// RemoveStagingMeta 删除 staging 文件元数据
func (m *CacheManager) RemoveStagingMeta(fid string) error {
	return m.DB.RemoveStagingMeta(fid)
}

// --- 以下为 CacheManager 的原有方法 ---

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
func (m *CacheManager) SavePendingNode(path, fid, parentFid, name, localPath string, size int64, isFolder bool, nonce []byte) error {
	return m.DB.SavePendingNode(path, fid, parentFid, name, localPath, size, isFolder, nonce)
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

	evicted := 0
	for _, c := range chunks {
		os.Remove(c.Path) // 忽略删除错误
		m.DB.DeleteChunk(c.Fid, c.Index)
		evicted++
	}

	return nil
}

// CleanupStagingMetas 清理过期的 staging 元数据和孤立文件
// abandonedMaxAge: abandoned 状态超过此时间则删除文件
func (m *CacheManager) CleanupStagingMetas(abandonedMaxAge time.Duration) error {
	// 获取孤立的 staging fid（没有 pending node 关联）
	pendingNodes, err := m.DB.GetPendingNodes()
	if err != nil {
		return err
	}

	orphanFids := []string{}
	activeFids := make(map[string]bool)
	for _, n := range pendingNodes {
		if n.Fid != "" {
			activeFids[n.Fid] = true
		}
	}

	// 找出孤立的 fid（staging_meta 中有但 pending_nodes 中没有）
	allMetas, err := m.DB.GetStagingMetasByStatus("active")
	if err != nil {
		return err
	}
	for _, meta := range allMetas {
		if !activeFids[meta.Fid] {
			orphanFids = append(orphanFids, meta.Fid)
		}
	}

	return m.DB.CleanupStagingMetas(abandonedMaxAge, orphanFids)
}

// Maintenance 执行数据库维护
func (m *CacheManager) Maintenance() error {
	// 清理 staging 元数据（删除 abandoned 超过 24h 的）
	if err := m.CleanupStagingMetas(24 * time.Hour); err != nil {
		fmt.Printf("CleanupStagingMetas failed: %v\n", err)
	}

	return m.DB.Maintenance()
}

// MaintenanceStart 在后台启动维护任务
func (m *CacheManager) MaintenanceStart() {
	go func() {
		if err := m.Maintenance(); err != nil {
			fmt.Printf("Background maintenance failed: %v\n", err)
		}
	}()
}

// Close 关闭缓存管理器
func (m *CacheManager) Close() error {
	return m.DB.Close()
}
