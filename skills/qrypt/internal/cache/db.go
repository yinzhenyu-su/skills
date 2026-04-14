package cache

import (
	"database/sql"
	_ "modernc.org/sqlite"
	"strings"
)

// CacheDB 管理缓存分块的元数据
type CacheDB struct {
	db *sql.DB
}

// NewCacheDB 初始化并返回数据库实例
func NewCacheDB(dbPath string) (*CacheDB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
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
		mtime DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS name_cache (
		fid TEXT PRIMARY KEY,
		encrypted_name TEXT,
		decrypted_name TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_name_cache_encrypted_name ON name_cache(encrypted_name);
	`
	_, err = db.Exec(query)
	if err != nil {
		return nil, err
	}

	// 简单的数据库迁移：如果 pending_nodes 表已经存在但没有 parent_fid 列，则添加它。
	_, _ = db.Exec("ALTER TABLE pending_nodes ADD COLUMN parent_fid TEXT")
	_, _ = db.Exec("ALTER TABLE pending_nodes ADD COLUMN local_path TEXT")

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

// SavePendingNode 持久化未完成的文件节点
func (c *CacheDB) SavePendingNode(path, fid, parentFid, name, localPath string, size int64, isFolder bool, nonce []byte) error {
	query := `INSERT OR REPLACE INTO pending_nodes (path, fid, parent_fid, name, local_path, size, is_folder, file_nonce) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := c.db.Exec(query, path, fid, parentFid, name, localPath, size, isFolder, nonce)
	return err
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
	Path      string
	Fid       string
	ParentFid string
	Name      string
	LocalPath string
	Size      int64
	IsFolder  bool
	Nonce     []byte
}

// GetPendingNodes 获取所有待同步的节点
func (c *CacheDB) GetPendingNodes() ([]PendingNode, error) {
	rows, err := c.db.Query("SELECT path, fid, parent_fid, name, local_path, size, is_folder, file_nonce FROM pending_nodes")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []PendingNode
	for rows.Next() {
		var n PendingNode
		if err := rows.Scan(&n.Path, &n.Fid, &n.ParentFid, &n.Name, &n.LocalPath, &n.Size, &n.IsFolder, &n.Nonce); err != nil {
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
