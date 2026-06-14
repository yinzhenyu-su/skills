package vfs

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yinzhenyu/skills/qrypt/cipher"
	"github.com/yinzhenyu/skills/qrypt/internal/mockdrive"
)

func newTestVFS(t *testing.T) *VFSImpl {
	t.Helper()
	drv := mockdrive.NewDriver()
	cp, err := cipher.NewRcloneCipher("test-password", "")
	require.NoError(t, err)
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	vfs := NewVFSImpl(drv, cp, cacheDir, FSOptions{
		MaxRetries:        2,
		ConcurrentUploads: 2,
		MemCacheSizeMB:    16,
		WriteBackTimeout:  50 * time.Millisecond,
	})
	return vfs
}

func TestVFSImpl_Stat_Root(t *testing.T) {
	t.Parallel()

	vfs := newTestVFS(t)
	info, err := vfs.Stat(context.Background(), "/")

	require.NoError(t, err)
	assert.True(t, info.IsDir, "root should be a directory")
	assert.Equal(t, "", info.Name, "root name should be empty")
}

func TestVFSImpl_Mkdir_Then_Stat(t *testing.T) {
	t.Parallel()

	vfs := newTestVFS(t)
	err := vfs.Mkdir(context.Background(), "/testdir")
	require.NoError(t, err)

	info, err := vfs.Stat(context.Background(), "/testdir")
	require.NoError(t, err)
	assert.True(t, info.IsDir, "testdir should be a directory")
	assert.Equal(t, "testdir", info.Name)
}

func TestVFSImpl_Create_Then_Stat(t *testing.T) {
	t.Parallel()

	vfs := newTestVFS(t)
	err := vfs.Create(context.Background(), "/file.txt")
	require.NoError(t, err)

	info, err := vfs.Stat(context.Background(), "/file.txt")
	require.NoError(t, err)
	assert.False(t, info.IsDir, "file.txt should not be a directory")
	assert.Equal(t, "file.txt", info.Name)
	assert.Equal(t, int64(0), info.Size, "new file should have size 0")
}

func TestVFSImpl_Write_Then_Read(t *testing.T) {
	t.Parallel()

	vfs := newTestVFS(t)
	err := vfs.Create(context.Background(), "/file.txt")
	require.NoError(t, err)

	content := []byte("hello world")
	err = vfs.Write(context.Background(), "/file.txt", 0, bytes.NewReader(content))
	require.NoError(t, err)

	rc, err := vfs.Read(context.Background(), "/file.txt", 0)
	require.NoError(t, err)
	defer rc.Close()

	readContent, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, content, readContent)
}

func TestVFSImpl_Rename(t *testing.T) {
	t.Parallel()

	vfs := newTestVFS(t)
	err := vfs.Create(context.Background(), "/old.txt")
	require.NoError(t, err)

	// File exists before rename
	_, err = vfs.Stat(context.Background(), "/old.txt")
	require.NoError(t, err)

	err = vfs.Rename(context.Background(), "/old.txt", "/new.txt")
	require.NoError(t, err)

	// New path exists
	info, err := vfs.Stat(context.Background(), "/new.txt")
	require.NoError(t, err)
	assert.Equal(t, "new.txt", info.Name)

	// Old path should not exist
	_, err = vfs.Stat(context.Background(), "/old.txt")
	assert.Error(t, err, "old path should not exist after rename")
}

func TestVFSImpl_Remove(t *testing.T) {
	t.Parallel()

	vfs := newTestVFS(t)
	err := vfs.Create(context.Background(), "/file.txt")
	require.NoError(t, err)

	// File exists before remove
	_, err = vfs.Stat(context.Background(), "/file.txt")
	require.NoError(t, err)

	err = vfs.Remove(context.Background(), "/file.txt")
	require.NoError(t, err)

	// File should not exist after remove
	_, err = vfs.Stat(context.Background(), "/file.txt")
	assert.Error(t, err, "file should not exist after remove")
}

func TestVFSImpl_ListDir_EmptyRoot(t *testing.T) {
	t.Parallel()

	vfs := newTestVFS(t)
	entries, err := vfs.ListDir(context.Background(), "/")

	require.NoError(t, err)
	// Root initially has no children; "." and ".." are FUSE-level entries
	// not tracked by the tree, so the list should be empty.
	assert.Len(t, entries, 0, "empty root should have no entries")
}

func TestVFSImpl_ListDir_WithChildren(t *testing.T) {
	t.Parallel()

	vfs := newTestVFS(t)
	err := vfs.Mkdir(context.Background(), "/subdir")
	require.NoError(t, err)

	err = vfs.Create(context.Background(), "/test.txt")
	require.NoError(t, err)

	entries, err := vfs.ListDir(context.Background(), "/")
	require.NoError(t, err)

	// Build a lookup map from the entries
	byName := make(map[string]bool)
	for _, e := range entries {
		byName[e.Name] = e.IsDir
	}

	isDir, ok := byName["subdir"]
	assert.True(t, ok, "subdir should be in listing")
	assert.True(t, isDir, "subdir should be a directory")

	isDir, ok = byName["test.txt"]
	assert.True(t, ok, "test.txt should be in listing")
	assert.False(t, isDir, "test.txt should not be a directory")
}

func TestVFSImpl_Truncate(t *testing.T) {
	t.Parallel()

	vfs := newTestVFS(t)
	err := vfs.Create(context.Background(), "/file.txt")
	require.NoError(t, err)

	err = vfs.Write(context.Background(), "/file.txt", 0, bytes.NewReader([]byte("hello world")))
	require.NoError(t, err)

	err = vfs.Truncate(context.Background(), "/file.txt", 3)
	require.NoError(t, err)

	info, err := vfs.Stat(context.Background(), "/file.txt")
	require.NoError(t, err)
	assert.Equal(t, int64(3), info.Size, "file should be truncated to 3 bytes")
}

func TestVFSImpl_Utimens(t *testing.T) {
	t.Parallel()

	vfs := newTestVFS(t)
	err := vfs.Create(context.Background(), "/file.txt")
	require.NoError(t, err)

	newTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	err = vfs.Utimens(context.Background(), "/file.txt", newTime)
	require.NoError(t, err)

	info, err := vfs.Stat(context.Background(), "/file.txt")
	require.NoError(t, err)
	assert.True(t, info.ModTime.Equal(newTime), "ModTime should match the set time")
}

func TestVFSImpl_Create_Write_Read_Stream(t *testing.T) {
	t.Parallel()

	vfs := newTestVFS(t)
	err := vfs.Create(context.Background(), "/large.txt")
	require.NoError(t, err)

	// ~100KB of data
	content := bytes.Repeat([]byte("ABCDEFGHIJ"), 10000)
	err = vfs.Write(context.Background(), "/large.txt", 0, bytes.NewReader(content))
	require.NoError(t, err)

	rc, err := vfs.Read(context.Background(), "/large.txt", 0)
	require.NoError(t, err)
	defer rc.Close()

	readContent, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, content, readContent)
}

func TestVFSImpl_Shutdown(t *testing.T) {
	t.Parallel()

	vfs := newTestVFS(t)
	// Should not panic
	vfs.Shutdown()
}
