package staging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

type Store struct {
	dir string
}

func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) Dir() string {
	return s.dir
}

const diskSpaceThresholdWarn = 1 << 30
const diskSpaceThresholdCrit = 100 << 20

func (s *Store) checkDiskSpace() error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(s.dir, &stat); err != nil {
		return err
	}
	blockSize := int64(stat.Bsize)
	if blockSize == 0 {
		blockSize = 4096
	}
	avail := int64(stat.Bavail) * blockSize
	if avail < diskSpaceThresholdCrit {
		return &ErrDiskSpaceCritical{avail}
	}
	return nil
}

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

func FidFromPath(path string) string {
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
	return n, nil
}

func (s *Store) ReadAt(path string, buf []byte, off int64) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return f.ReadAt(buf, off)
}

func (s *Store) OpenReader(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func (s *Store) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

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
	return nil
}

func (s *Store) Sync(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func (s *Store) Truncate(path string, size int64) error {
	if err := s.Ensure(path); err != nil {
		return err
	}
	return os.Truncate(path, size)
}

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
		if filepath.Ext(name) == ".staging" {
			files = append(files, filepath.Join(s.dir, name))
		}
	}
	return files, nil
}

func (s *Store) CleanupOrphanedStagingFiles(activeFids map[string]bool) ([]string, error) {
	files, err := s.ListStagingFiles()
	if err != nil {
		return nil, err
	}
	var cleaned []string
	for _, path := range files {
		fid := FidFromPath(path)
		if !activeFids[fid] {
			if err := s.Remove(path); err != nil {
				continue
			}
			cleaned = append(cleaned, path)
		}
	}
	return cleaned, nil
}
