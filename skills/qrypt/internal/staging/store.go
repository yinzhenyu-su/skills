package staging

import (
	"io"
	"os"
	"path/filepath"
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

func (s *Store) Path(fid string) string {
	return filepath.Join(s.dir, fid+".staging")
}

func (s *Store) Create(fid string) (string, error) {
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
