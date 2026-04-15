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
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
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
	return f.WriteAt(data, off)
}

func (s *Store) Truncate(path string, size int64) error {
	if err := s.Ensure(path); err != nil {
		return err
	}
	return os.Truncate(path, size)
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

func (s *Store) Remove(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *Store) Snapshot(path string) (string, error) {
	src, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer src.Close()

	dst, err := os.CreateTemp(s.dir, filepath.Base(path)+".upload-*")
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		_ = os.Remove(dst.Name())
		return "", err
	}
	return dst.Name(), nil
}
