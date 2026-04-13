package cache

import (
	"database/sql"
	_ "modernc.org/sqlite"
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
	`
	_, err = db.Exec(query)
	if err != nil {
		return nil, err
	}

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
func (c *CacheDB) GetOldestChunks(targetSize int64) ([]struct{Fid string; Index int64; Path string; Size int64}, error) {
	rows, err := c.db.Query("SELECT fid, chunk_index, file_path, size FROM chunks WHERE is_dirty = 0 ORDER BY access_time ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deletedSize int64
	var chunks []struct{Fid string; Index int64; Path string; Size int64}

	for rows.Next() {
		var cinfo struct{Fid string; Index int64; Path string; Size int64}
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

// Close 关闭数据库
func (c *CacheDB) Close() error {
	return c.db.Close()
}
