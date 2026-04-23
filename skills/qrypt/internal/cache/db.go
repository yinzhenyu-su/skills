package cache

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// CacheDB 管理缓存分块的元数据
type CacheDB struct {
	db *sql.DB
}

// NewCacheDB 初始化并返回数据库实例
func NewCacheDB(dbPath string) (*CacheDB, error) {
	db, err := sql.Open("sqlite", dbPath+"?_busy_timeout=10000")
	if err != nil {
		return nil, err
	}

	// 启用 WAL 模式：允许读写并发，提升高并发场景下的性能
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to set WAL mode: %w", err)
	}

	// 创建元数据表
	query := `
	CREATE TABLE IF NOT EXISTS chunks (
		fid TEXT,
		chunk_index INTEGER,
		file_path TEXT,
		size INTEGER,
		access_time DATETIME DEFAULT CURRENT_TIMESTAMP,
		is_dirty BOOLEAN DEFAULT 0,
		PRIMARY KEY (fid, chunk_index)
	);
	CREATE INDEX IF NOT EXISTS idx_access_time ON chunks(access_time);

	CREATE TABLE IF NOT EXISTS pending_nodes (
		path TEXT PRIMARY KEY,
		fid TEXT,
		parent_fid TEXT,
		name TEXT,
		local_path TEXT,
		size INTEGER,
		is_folder BOOLEAN,
		file_nonce BLOB,
		mtime DATETIME DEFAULT CURRENT_TIMESTAMP,
		base_server_mtime INTEGER DEFAULT 0,
		base_server_size INTEGER DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS ops_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		op_type TEXT,
		source_path TEXT,
		target_path TEXT,
		payload TEXT,
		status TEXT DEFAULT 'PENDING',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS name_cache (
		fid TEXT PRIMARY KEY,
		encrypted_name TEXT,
		decrypted_name TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_name_cache_encrypted_name ON name_cache(encrypted_name);

	CREATE TABLE IF NOT EXISTS staging_meta (
		fid TEXT PRIMARY KEY,
		local_path TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		size INTEGER DEFAULT 0,
		status TEXT DEFAULT 'active'
	);
	`
	_, err = db.Exec(query)
	if err != nil {
		return nil, err
	}

	// 简单的数据库迁移：如果 pending_nodes 表已经存在但没有对应列，则添加它。
	_, _ = db.Exec("ALTER TABLE pending_nodes ADD COLUMN parent_fid TEXT")
	_, _ = db.Exec("ALTER TABLE pending_nodes ADD COLUMN local_path TEXT")
	_, _ = db.Exec("ALTER TABLE pending_nodes ADD COLUMN base_server_mtime INTEGER DEFAULT 0")
	_, _ = db.Exec("ALTER TABLE pending_nodes ADD COLUMN base_server_size INTEGER DEFAULT 0")

	return &CacheDB{db: db}, nil
}

// UpdateAccessTime 更新分块的访问时间 (LRU)
func (c *CacheDB) UpdateAccessTime(fid string, chunkIndex int64) error {
	query := `UPDATE chunks SET access_time = CURRENT_TIMESTAMP WHERE fid = ? AND chunk_index = ?`
	_, err := c.db.Exec(query, fid, chunkIndex)
	return err
}

// GetChunk 获取分块信息
func (c *CacheDB) GetChunk(fid string, chunkIndex int64) (string, bool, error) {
	query := `SELECT file_path FROM chunks WHERE fid = ? AND chunk_index = ?`
	var filePath string
	err := c.db.QueryRow(query, fid, chunkIndex).Scan(&filePath)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return filePath, true, nil
}

// InsertChunk 插入新分块
func (c *CacheDB) InsertChunk(fid string, chunkIndex int64, filePath string, size int64, isDirty bool) error {
	query := `INSERT OR REPLACE INTO chunks (fid, chunk_index, file_path, size, is_dirty, access_time)
			  VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`
	_, err := c.db.Exec(query, fid, chunkIndex, filePath, size, isDirty)
	return err
}

// GetTotalSize 获取当前缓存总大小
func (c *CacheDB) GetTotalSize() (int64, error) {
	var total int64
	err := c.db.QueryRow("SELECT COALESCE(SUM(size), 0) FROM chunks").Scan(&total)
	return total, err
}

// GetOldestChunks 获取最久未访问的分块，直到达到目标大小
func (c *CacheDB) GetOldestChunks(targetSize int64) ([]struct {
	Fid   string
	Index int64
	Path  string
	Size  int64
}, error) {
	rows, err := c.db.Query("SELECT fid, chunk_index, file_path, size FROM chunks WHERE is_dirty = 0 ORDER BY access_time ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deletedSize int64
	var chunks []struct {
		Fid   string
		Index int64
		Path  string
		Size  int64
	}

	for rows.Next() {
		var cinfo struct {
			Fid   string
			Index int64
			Path  string
			Size  int64
		}
		if err := rows.Scan(&cinfo.Fid, &cinfo.Index, &cinfo.Path, &cinfo.Size); err != nil {
			return nil, err
		}
		chunks = append(chunks, cinfo)
		deletedSize += cinfo.Size
		if deletedSize >= targetSize {
			break
		}
	}
	return chunks, nil
}

// DeleteChunk 从元数据中删除分块
func (c *CacheDB) DeleteChunk(fid string, chunkIndex int64) error {
	_, err := c.db.Exec("DELETE FROM chunks WHERE fid = ? AND chunk_index = ?", fid, chunkIndex)
	return err
}

// SavePendingNode 持久化未完成的文件节点（含 SQLITE_BUSY 重试）
func (c *CacheDB) SavePendingNode(path, fid, parentFid, name, localPath string, size int64, isFolder bool, nonce []byte, baseMtime, baseSize int64) error {
	query := `INSERT OR REPLACE INTO pending_nodes (path, fid, parent_fid, name, local_path, size, is_folder, file_nonce, base_server_mtime, base_server_size) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		if attempt > 0 {
			// 指数退避：10ms, 20ms, 40ms, 80ms, 160ms, 320ms, 640ms, 1280ms, 2560ms
			delay := time.Duration(10<<uint(attempt-1)) * time.Millisecond
			time.Sleep(delay)
		}
		_, err := c.db.Exec(query, path, fid, parentFid, name, localPath, size, isFolder, nonce, baseMtime, baseSize)
		if err == nil {
			return nil
		}
		// Check if it's a SQLITE_BUSY error
		if !isSQLiteBusy(err) {
			return err
		}
		lastErr = err
	}
	return fmt.Errorf("SavePendingNode exceeded retries: %w", lastErr)
}

func isSQLiteBusy(err error) bool {
	return err != nil && strings.Contains(err.Error(), "database is locked")
}

// RemovePendingNode 移除已完成的文件节点
func (c *CacheDB) RemovePendingNode(path string) error {
	_, err := c.db.Exec("DELETE FROM pending_nodes WHERE path = ?", path)
	return err
}

// RemovePendingNodesByPrefix 按路径前缀移除待同步节点
func (c *CacheDB) RemovePendingNodesByPrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	like := prefix
	if !strings.HasSuffix(like, "/") {
		like += "/"
	}
	like += "%"
	_, err := c.db.Exec("DELETE FROM pending_nodes WHERE path = ? OR path LIKE ?", prefix, like)
	return err
}

// RemovePendingNodesByFid 按 fid 移除待同步节点
func (c *CacheDB) RemovePendingNodesByFid(fid string) error {
	if fid == "" {
		return nil
	}
	_, err := c.db.Exec("DELETE FROM pending_nodes WHERE fid = ?", fid)
	return err
}

// PendingNode 定义待同步的节点
type PendingNode struct {
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

// GetPendingNodes 获取所有待同步的节点
func (c *CacheDB) GetPendingNodes() ([]PendingNode, error) {
	rows, err := c.db.Query("SELECT path, fid, parent_fid, name, local_path, size, is_folder, file_nonce, base_server_mtime, base_server_size FROM pending_nodes")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []PendingNode
	for rows.Next() {
		var n PendingNode
		if err := rows.Scan(&n.Path, &n.Fid, &n.ParentFid, &n.Name, &n.LocalPath, &n.Size, &n.IsFolder, &n.Nonce, &n.BaseServerMtime, &n.BaseServerSize); err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

// GetChunkPathsByFid 获取某个文件 fid 关联的本地 chunk 文件路径
func (c *CacheDB) GetChunkPathsByFid(fid string) ([]string, error) {
	rows, err := c.db.Query("SELECT file_path FROM chunks WHERE fid = ?", fid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, nil
}

// DeleteChunksByFid 按 fid 删除 chunk 元数据
func (c *CacheDB) DeleteChunksByFid(fid string) error {
	if fid == "" {
		return nil
	}
	_, err := c.db.Exec("DELETE FROM chunks WHERE fid = ?", fid)
	return err
}

// GetDirtyChunks 获取文件的所有脏分块索引
func (c *CacheDB) GetDirtyChunks(fid string) ([]int64, error) {
	rows, err := c.db.Query("SELECT chunk_index FROM chunks WHERE fid = ? AND is_dirty = 1", fid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var indices []int64
	for rows.Next() {
		var idx int64
		if err := rows.Scan(&idx); err != nil {
			return nil, err
		}
		indices = append(indices, idx)
	}
	return indices, nil
}

// OpsLogEntry 表示一个元数据操作日志
type OpsLogEntry struct {
	ID         int64
	OpType     string
	SourcePath string
	TargetPath string
	Payload    string
	Status     string
	CreatedAt  time.Time
}

// AddOpsLogEntry 添加一条操作日志
func (c *CacheDB) AddOpsLogEntry(opType, sourcePath, targetPath, payload string) (int64, error) {
	query := `INSERT INTO ops_log (op_type, source_path, target_path, payload) VALUES (?, ?, ?, ?)`
	res, err := c.db.Exec(query, opType, sourcePath, targetPath, payload)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateOpsLogStatus 更新操作日志状态
func (c *CacheDB) UpdateOpsLogStatus(id int64, status string) error {
	query := `UPDATE ops_log SET status = ? WHERE id = ?`
	_, err := c.db.Exec(query, status, id)
	return err
}

// GetPendingOpsLogs 获取所有待处理的操作日志
func (c *CacheDB) GetPendingOpsLogs() ([]OpsLogEntry, error) {
	query := `SELECT id, op_type, source_path, target_path, payload, status, created_at FROM ops_log WHERE status = 'PENDING' ORDER BY created_at ASC`
	rows, err := c.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []OpsLogEntry
	for rows.Next() {
		var l OpsLogEntry
		if err := rows.Scan(&l.ID, &l.OpType, &l.SourcePath, &l.TargetPath, &l.Payload, &l.Status, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, nil
}

// GetCachedName 通过 fid 与加密名读取解密名缓存
func (c *CacheDB) GetCachedName(fid, encryptedName string) (string, bool, error) {
	query := `SELECT decrypted_name FROM name_cache WHERE fid = ? AND encrypted_name = ?`
	var decrypted string
	err := c.db.QueryRow(query, fid, encryptedName).Scan(&decrypted)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return decrypted, true, nil
}

// SaveCachedName 写入或更新解密名缓存
func (c *CacheDB) SaveCachedName(fid, encryptedName, decryptedName string) error {
	query := `INSERT INTO name_cache (fid, encrypted_name, decrypted_name, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(fid) DO UPDATE SET
			encrypted_name = excluded.encrypted_name,
			decrypted_name = excluded.decrypted_name,
			updated_at = CURRENT_TIMESTAMP`
	_, err := c.db.Exec(query, fid, encryptedName, decryptedName)
	return err
}

// Close 关闭数据库
func (c *CacheDB) Close() error {
	return c.db.Close()
}

// Maintenance 执行数据库维护：清理过期元数据并压缩空间
func (c *CacheDB) Maintenance() error {
	// 1. 删除 30 天未访问的非脏分块
	result, err := c.db.Exec(`
		DELETE FROM chunks
		WHERE is_dirty = 0
		AND access_time < datetime('now', '-30 days')
	`)
	if err != nil {
		return fmt.Errorf("failed to delete expired chunks: %w", err)
	}
	deleted, _ := result.RowsAffected()

	// 2. 如果删除了大量数据，执行 VACUUM
	if deleted > 100 {
		if _, err := c.db.Exec("PRAGMA incremental_vacuum"); err != nil {
			// fallback to regular VACUUM if incremental not supported
			if _, err2 := c.db.Exec("VACUUM"); err2 != nil {
				return fmt.Errorf("vacuum failed: %w (incremental: %v)", err2, err)
			}
		}
	}

	// 3. 优化查询计划 (非致命，忽略错误)
	_, _ = c.db.Exec("PRAGMA optimize")

	return nil
}

// MaintenanceStart 在后台启动维护任务
func (c *CacheDB) MaintenanceStart() {
	go func() {
		if err := c.Maintenance(); err != nil {
			fmt.Printf("Background maintenance failed: %v\n", err)
		}
	}()
}

// --- Staging Meta 管理 ---

// StagingMeta 表示一个 staging 文件的元数据
type StagingMeta struct {
	Fid       string
	LocalPath string
	CreatedAt time.Time
	UpdatedAt time.Time
	Size      int64
	Status    string // "active", "syncing", "abandoned"
}

// SaveStagingMeta 保存或更新 staging 文件元数据
func (c *CacheDB) SaveStagingMeta(fid, localPath string, size int64) error {
	query := `INSERT INTO staging_meta (fid, local_path, size, updated_at, status)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, 'active')
		ON CONFLICT(fid) DO UPDATE SET
			local_path = excluded.local_path,
			size = excluded.size,
			updated_at = CURRENT_TIMESTAMP,
			status = CASE WHEN status = 'abandoned' THEN 'active' ELSE status END`
	_, err := c.db.Exec(query, fid, localPath, size)
	return err
}

// UpdateStagingMeta 更新 staging 文件的 size 和 updated_at
func (c *CacheDB) UpdateStagingMeta(fid string, size int64) error {
	query := `UPDATE staging_meta SET size = ?, updated_at = CURRENT_TIMESTAMP WHERE fid = ?`
	_, err := c.db.Exec(query, size, fid)
	return err
}

// GetStagingMeta 获取 staging 文件元数据
func (c *CacheDB) GetStagingMeta(fid string) (*StagingMeta, error) {
	query := `SELECT fid, local_path, created_at, updated_at, size, status FROM staging_meta WHERE fid = ?`
	var m StagingMeta
	err := c.db.QueryRow(query, fid).Scan(&m.Fid, &m.LocalPath, &m.CreatedAt, &m.UpdatedAt, &m.Size, &m.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// GetStagingMetasByStatus 获取指定状态的所有 staging 元数据
func (c *CacheDB) GetStagingMetasByStatus(status string) ([]StagingMeta, error) {
	query := `SELECT fid, local_path, created_at, updated_at, size, status FROM staging_meta WHERE status = ?`
	rows, err := c.db.Query(query, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metas []StagingMeta
	for rows.Next() {
		var m StagingMeta
		if err := rows.Scan(&m.Fid, &m.LocalPath, &m.CreatedAt, &m.UpdatedAt, &m.Size, &m.Status); err != nil {
			return nil, err
		}
		metas = append(metas, m)
	}
	return metas, nil
}

// UpdateStagingStatus 更新 staging 文件状态
func (c *CacheDB) UpdateStagingStatus(fid, status string) error {
	query := `UPDATE staging_meta SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE fid = ?`
	_, err := c.db.Exec(query, status, fid)
	return err
}

// RemoveStagingMeta 删除 staging 元数据
func (c *CacheDB) RemoveStagingMeta(fid string) error {
	_, err := c.db.Exec("DELETE FROM staging_meta WHERE fid = ?", fid)
	return err
}

// CleanupStagingMetas 清理过期的 staging 记录和孤立文件
// maxAge: abandoned 状态超过此时间则删除
// orphanStagingFiles: staging 目录中存在的文件但没有对应 pending node
func (c *CacheDB) CleanupStagingMetas(maxAge time.Duration, orphanFids []string) error {
	cutoff := time.Now().Add(-maxAge)
	cutoffStr := cutoff.Format("2006-01-02 15:04:05")

	// 1. 删除 abandoned 超过 maxAge 的记录
	rows, err := c.db.Query(`
		SELECT fid, local_path FROM staging_meta
		WHERE status = 'abandoned' AND updated_at < ?
	`, cutoffStr)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var fid, localPath string
		if err := rows.Scan(&fid, &localPath); err != nil {
			continue
		}
		// 删除物理文件
		if localPath != "" {
			os.Remove(localPath)
		}
		// 删除元数据
		c.RemoveStagingMeta(fid)
	}

	// 2. 删除孤立的 staging 文件（无 pending node 关联）
	for _, fid := range orphanFids {
		meta, _ := c.GetStagingMeta(fid)
		if meta != nil && meta.Status == "active" {
			// 标记为 abandoned，等待下次清理
			c.UpdateStagingStatus(fid, "abandoned")
		}
	}

	return nil
}

// OpsLogData 用于批量添加时的简易结构
type OpsLogData struct {
	OpType     string
	SourcePath string
	TargetPath string
	Payload    string
}

// BatchAddOpsLog 批量添加操作日志
func (c *CacheDB) BatchAddOpsLog(logs []OpsLogData) error {
	if len(logs) == 0 {
		return nil
	}
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("INSERT INTO ops_log (op_type, source_path, target_path, payload) VALUES (?, ?, ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, l := range logs {
		_, _ = stmt.Exec(l.OpType, l.SourcePath, l.TargetPath, l.Payload)
	}
	return tx.Commit()
}

// BatchMarkOpsDone 批量标记操作日志为完成
func (c *CacheDB) BatchMarkOpsDone(fids []string, paths []string) error {
	if len(fids) == 0 && len(paths) == 0 {
		return nil
	}
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if len(fids) > 0 {
		stmt, err := tx.Prepare("UPDATE ops_log SET status = 'DONE' WHERE status = 'PENDING' AND payload LIKE ?")
		if err == nil {
			for _, fid := range fids {
				if fid != "" {
					// 核心修复：允许 local_ FID 的任务被标记完成
					_, _ = stmt.Exec("%" + fid + "%")
				}
			}
			stmt.Close()
		}
	}

	if len(paths) > 0 {
		stmt, err := tx.Prepare("UPDATE ops_log SET status = 'DONE' WHERE status = 'PENDING' AND source_path = ?")
		if err == nil {
			for _, p := range paths {
				_, _ = stmt.Exec(p)
			}
			stmt.Close()
		}
	}

	return tx.Commit()
}

// BatchDeleteNodeState 在一个事务中批量删除多个节点的元数据记录
func (c *CacheDB) BatchDeleteNodeState(fids []string, paths []string) error {
	if len(fids) == 0 && len(paths) == 0 {
		return nil
	}

	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if len(paths) > 0 {
		stmt, err := tx.Prepare("DELETE FROM pending_nodes WHERE path = ?")
		if err == nil {
			for _, p := range paths {
				_, _ = stmt.Exec(p)
			}
			stmt.Close()
		}
	}

	if len(fids) > 0 {
		stmt1, _ := tx.Prepare("DELETE FROM pending_nodes WHERE fid = ?")
		stmt2, _ := tx.Prepare("DELETE FROM chunks WHERE fid = ?")
		stmt3, _ := tx.Prepare("DELETE FROM staging_meta WHERE fid = ?")
		stmt4, _ := tx.Prepare("DELETE FROM name_cache WHERE fid = ?")
		
		for _, f := range fids {
			if f == "" {
				continue
			}
			// 核心修复：不再跳过 local_ 前缀，确保清理 staging_meta 和 pending_nodes 中的本地任务
			if stmt1 != nil { _, _ = stmt1.Exec(f) }
			if stmt2 != nil { _, _ = stmt2.Exec(f) }
			if stmt3 != nil { _, _ = stmt3.Exec(f) }
			if stmt4 != nil { _, _ = stmt4.Exec(f) }
		}
		if stmt1 != nil { stmt1.Close() }
		if stmt2 != nil { stmt2.Close() }
		if stmt3 != nil { stmt3.Close() }
		if stmt4 != nil { stmt4.Close() }
	}

	return tx.Commit()
}
