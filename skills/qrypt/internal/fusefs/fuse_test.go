//go:build !nofuse

package fusefs

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/winfsp/cgofuse/fuse"

	vfspkg "github.com/yinzhenyu/skills/qrypt/pkg/vfs"
)

// mockVFS implements vfs.VFS for testing
type mockVFS struct {
	statFunc    func(ctx context.Context, path string) (vfspkg.NodeInfo, error)
	listDirFunc func(ctx context.Context, path string) ([]vfspkg.DirEntry, error)
	readFunc    func(ctx context.Context, path string, offset int64) (io.ReadCloser, error)
	createFunc  func(ctx context.Context, path string) error
	writeFunc   func(ctx context.Context, path string, offset int64, reader io.Reader) error
	mkdirFunc   func(ctx context.Context, path string) error
	removeFunc  func(ctx context.Context, path string) error
	renameFunc  func(ctx context.Context, oldPath, newPath string) error
	truncateFunc func(ctx context.Context, path string, size int64) error
	utimensFunc func(ctx context.Context, path string, mtime time.Time) error
	flushFunc   func(ctx context.Context, path string) error
	fsyncFunc   func(ctx context.Context, path string) error
	shutdownFunc func()
}

func (m *mockVFS) Stat(ctx context.Context, path string) (vfspkg.NodeInfo, error) {
	if m.statFunc != nil {
		return m.statFunc(ctx, path)
	}
	return vfspkg.NodeInfo{}, errors.New("not implemented")
}

func (m *mockVFS) ListDir(ctx context.Context, path string) ([]vfspkg.DirEntry, error) {
	if m.listDirFunc != nil {
		return m.listDirFunc(ctx, path)
	}
	return nil, errors.New("not implemented")
}

func (m *mockVFS) Read(ctx context.Context, path string, offset int64) (io.ReadCloser, error) {
	if m.readFunc != nil {
		return m.readFunc(ctx, path, offset)
	}
	return nil, errors.New("not implemented")
}

func (m *mockVFS) Create(ctx context.Context, path string) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, path)
	}
	return errors.New("not implemented")
}

func (m *mockVFS) Write(ctx context.Context, path string, offset int64, reader io.Reader) error {
	if m.writeFunc != nil {
		return m.writeFunc(ctx, path, offset, reader)
	}
	return errors.New("not implemented")
}

func (m *mockVFS) Mkdir(ctx context.Context, path string) error {
	if m.mkdirFunc != nil {
		return m.mkdirFunc(ctx, path)
	}
	return errors.New("not implemented")
}

func (m *mockVFS) Remove(ctx context.Context, path string) error {
	if m.removeFunc != nil {
		return m.removeFunc(ctx, path)
	}
	return errors.New("not implemented")
}

func (m *mockVFS) Rename(ctx context.Context, oldPath, newPath string) error {
	if m.renameFunc != nil {
		return m.renameFunc(ctx, oldPath, newPath)
	}
	return errors.New("not implemented")
}

func (m *mockVFS) Truncate(ctx context.Context, path string, size int64) error {
	if m.truncateFunc != nil {
		return m.truncateFunc(ctx, path, size)
	}
	return errors.New("not implemented")
}

func (m *mockVFS) Utimens(ctx context.Context, path string, mtime time.Time) error {
	if m.utimensFunc != nil {
		return m.utimensFunc(ctx, path, mtime)
	}
	return errors.New("not implemented")
}

func (m *mockVFS) Flush(ctx context.Context, path string) error {
	if m.flushFunc != nil {
		return m.flushFunc(ctx, path)
	}
	return errors.New("not implemented")
}

func (m *mockVFS) Fsync(ctx context.Context, path string) error {
	if m.fsyncFunc != nil {
		return m.fsyncFunc(ctx, path)
	}
	return errors.New("not implemented")
}

func (m *mockVFS) Shutdown() {
	if m.shutdownFunc != nil {
		m.shutdownFunc()
	}
}

var testTime = time.Date(2025, 6, 14, 12, 0, 0, 0, time.UTC)

func TestFUSEAdapter_Getattr_File(t *testing.T) {
	mock := &mockVFS{
		statFunc: func(ctx context.Context, path string) (vfspkg.NodeInfo, error) {
			return vfspkg.NodeInfo{
				Name:    "test.txt",
				IsDir:   false,
				Size:    100,
				ModTime: testTime,
			}, nil
		},
	}
	adapter := NewFUSEAdapter(mock)

	var stat fuse.Stat_t
	errc := adapter.Getattr("/test.txt", &stat, 0)

	assert.Equal(t, 0, errc)
	assert.Equal(t, int64(100), stat.Size)
	assert.True(t, stat.Mode&fuse.S_IFREG != 0, "expected S_IFREG bit in mode")
	assert.Equal(t, uint32(1), stat.Nlink)
}

func TestFUSEAdapter_Getattr_Directory(t *testing.T) {
	mock := &mockVFS{
		statFunc: func(ctx context.Context, path string) (vfspkg.NodeInfo, error) {
			return vfspkg.NodeInfo{
				Name:    "dir",
				IsDir:   true,
				Size:    0,
				ModTime: testTime,
			}, nil
		},
	}
	adapter := NewFUSEAdapter(mock)

	var stat fuse.Stat_t
	errc := adapter.Getattr("/dir", &stat, 0)

	assert.Equal(t, 0, errc)
	assert.True(t, stat.Mode&fuse.S_IFDIR != 0, "expected S_IFDIR bit in mode")
	assert.Equal(t, uint32(2), stat.Nlink)
}

func TestFUSEAdapter_Getattr_NotFound(t *testing.T) {
	mock := &mockVFS{
		statFunc: func(ctx context.Context, path string) (vfspkg.NodeInfo, error) {
			return vfspkg.NodeInfo{}, errors.New("not found")
		},
	}
	adapter := NewFUSEAdapter(mock)

	var stat fuse.Stat_t
	errc := adapter.Getattr("/nonexistent", &stat, 0)

	assert.Equal(t, -fuse.ENOENT, errc)
}

func TestFUSEAdapter_Readdir(t *testing.T) {
	mock := &mockVFS{
		listDirFunc: func(ctx context.Context, path string) ([]vfspkg.DirEntry, error) {
			return []vfspkg.DirEntry{
				{Name: "a.txt", IsDir: false},
				{Name: "subdir", IsDir: true},
			}, nil
		},
	}
	adapter := NewFUSEAdapter(mock)

	var entries []string
	fill := func(name string, stat *fuse.Stat_t, ofst int64) bool {
		entries = append(entries, name)
		return true
	}
	errc := adapter.Readdir("/", fill, 0, 0)

	assert.Equal(t, 0, errc)
	assert.Equal(t, []string{".", "..", "a.txt", "subdir"}, entries)
}

func TestFUSEAdapter_Mkdir(t *testing.T) {
	mock := &mockVFS{
		mkdirFunc: func(ctx context.Context, path string) error {
			return nil
		},
	}
	adapter := NewFUSEAdapter(mock)

	errc := adapter.Mkdir("/newdir", 0755)
	assert.Equal(t, 0, errc)
}

func TestFUSEAdapter_Mkdir_Error(t *testing.T) {
	mock := &mockVFS{
		mkdirFunc: func(ctx context.Context, path string) error {
			return errors.New("exists")
		},
	}
	adapter := NewFUSEAdapter(mock)

	errc := adapter.Mkdir("/existing", 0755)
	assert.Equal(t, -fuse.EIO, errc)
}

func TestFUSEAdapter_Unlink(t *testing.T) {
	mock := &mockVFS{
		removeFunc: func(ctx context.Context, path string) error {
			return nil
		},
	}
	adapter := NewFUSEAdapter(mock)

	errc := adapter.Unlink("/file.txt")
	assert.Equal(t, 0, errc)
}

func TestFUSEAdapter_Shutdown(t *testing.T) {
	mock := &mockVFS{
		mkdirFunc: func(ctx context.Context, path string) error {
			return nil
		},
	}
	adapter := NewFUSEAdapter(mock)
	adapter.Shutdown()

	assert.True(t, adapter.IsShuttingDown(), "expected IsShuttingDown to be true after Shutdown")

	// Subsequent operations should return -fuse.EIO
	errc := adapter.Mkdir("/anything", 0755)
	assert.Equal(t, -fuse.EIO, errc)
}
