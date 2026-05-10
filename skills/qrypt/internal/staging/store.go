package staging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// Store manages file-level local staging files used by the write-back path.
type Store struct {
	dir string
	// meta 是一个回调接口，用于更新 staging 文件元数据
	// 由外部（如 CacheManager）注入，避免循环依赖
	metaStore MetaStore
}

// MetaStore 定义元数据存储接口
type MetaStore interface {
	SaveStagingMeta(fid, localPath string, size int64) error
	UpdateStagingMeta(fid string, size int64) error
	RemoveStagingMeta(fid string) error
}

// NewStore 初始化 staging 目录
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// SetMetaStore 设置元数据存储接口
func (s *Store) SetMetaStore(meta MetaStore) {
	s.metaStore = meta
}

// Dir 返回 staging 目录路径
func (s *Store) Dir() string {
	return s.dir
}

// diskSpaceThresholdWarn 磁盘空间警告阈值 (1GB)
const diskSpaceThresholdWarn = 1 << 30
// diskSpaceThresholdCrit 磁盘空间 critical 阈值 (100MB)
const diskSpaceThresholdCrit = 100 << 20

// checkDiskSpace 检查磁盘空间，返回 error 如果空间不足
func (s *Store) checkDiskSpace() error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(s.dir, &stat); err != nil {
		return err
	}
	// Bsize is the block size on macOS; Frsize may not be available
	blockSize := int64(stat.Bsize)
	if blockSize == 0 {
		blockSize = 4096 // fallback
	}
	avail := int64(stat.Bavail) * blockSize
	if avail < diskSpaceThresholdCrit {
		return &ErrDiskSpaceCritical{avail}
	}
	return nil
}

// ErrDiskSpaceCritical 磁盘空间严重不足
type ErrDiskSpaceCritical struct {
	Available int64
}

func (e *ErrDiskSpaceCritical) Error() string {
	return "critical low disk space: available " + formatBytes(e.Available)
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	exp := 0
	for n >= unit {
		n /= unit
		exp++
	}
	return fmt.Sprintf("%d %cB", n, "KMGTPE"[exp])
}

func (s *Store) Path(fid string) string {
	return filepath.Join(s.dir, fid+".staging")
}

// fidFromPath 从 staging 文件路径提取 fid
func (s *Store) fidFromPath(path string) string {
	base := filepath.Base(path)
	if filepath.Ext(base) == ".staging" {
		return base[:len(base)-len(".staging")]
	}
	return base
}

func (s *Store) Create(fid string) (string, error) {
	if err := s.checkDiskSpace(); err != nil {
		return "", err
	}
	path := s.Path(fid)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}

	// 更新元数据
	if s.metaStore != nil {
		s.metaStore.SaveStagingMeta(fid, path, 0)
	}

	return path, nil
}

func (s *Store) Ensure(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

func (s *Store) WriteAt(path string, data []byte, off int64) (int, error) {
	if err := s.checkDiskSpace(); err != nil {
		return 0, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	n, err := f.WriteAt(data, off)
	if err != nil {
		return n, err
	}

	// 更新元数据
	if s.metaStore != nil && n > 0 {
		fid := s.fidFromPath(path)
		// 获取当前文件大小
		info, err := f.Stat()
		if err == nil {
			s.metaStore.UpdateStagingMeta(fid, info.Size())
		}
	}

	return n, nil
}

func (s *Store) Truncate(path string, size int64) error {
	if err := s.Ensure(path); err != nil {
		return err
	}
	if err := os.Truncate(path, size); err != nil {
		return err
	}

	// 更新元数据
	if s.metaStore != nil {
		fid := s.fidFromPath(path)
		s.metaStore.UpdateStagingMeta(fid, size)
	}

	return nil
}

func (s *Store) OpenReader(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func (s *Store) ReadAt(path string, buf []byte, off int64) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return f.ReadAt(buf, off)
}

func (s *Store) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// FileSize returns the size of a file at the given path.
func (s *Store) FileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (s *Store) Remove(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	// 删除元数据
	if s.metaStore != nil {
		fid := s.fidFromPath(path)
		s.metaStore.RemoveStagingMeta(fid)
	}

	return nil
}

// ListStagingFiles 列出 staging 目录中的所有文件
func (s *Store) ListStagingFiles() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// 跳过上传过程中的临时文件
		if filepath.Ext(name) == ".staging" {
			files = append(files, filepath.Join(s.dir, name))
		}
	}
	return files, nil
}

// CleanupOrphanedStagingFiles 清理孤立的 staging 文件
// activeFids: 当前活跃（有 pending node 关联）的 fid 列表
// 返回被清理的孤立文件路径列表
func (s *Store) CleanupOrphanedStagingFiles(activeFids map[string]bool) ([]string, error) {
	files, err := s.ListStagingFiles()
	if err != nil {
		return nil, err
	}

	var cleaned []string
	for _, path := range files {
		fid := s.fidFromPath(path)
		if !activeFids[fid] {
			// 没有对应的 pending node，删除孤立文件
			if err := s.Remove(path); err != nil {
				continue
			}
			cleaned = append(cleaned, path)
		}
	}
	return cleaned, nil
}
