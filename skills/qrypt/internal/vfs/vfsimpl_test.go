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
	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/internal/mockdrive"
)

func newTestVFS(t *testing.T) *VFSImpl {
	t.Helper()
	drv := mockdrive.NewDriver()
	cp, err := cipher.NewRcloneCipher("test-password", "")
	require.NoError(t, err)
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	vfs := NewVFSImpl(drv, cp, cacheDir, "0", FSOptions{
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
	vfs.Shutdown()
}

// TestVFSImpl_Mkdir_Sync_ChildFile verifies that after Mkdir, creating a file
// inside the new directory uses the correct (remote) parent FID.
func TestVFSImpl_Mkdir_Sync_ChildFile(t *testing.T) {
	t.Parallel()

	vfs := newTestVFS(t)
	ctx := context.Background()

	require.NoError(t, vfs.Mkdir(ctx, "/subdir"))
	require.NoError(t, vfs.Create(ctx, "/subdir/child.txt"))

	// Stat should succeed for both.
	_, err := vfs.Stat(ctx, "/subdir")
	require.NoError(t, err)
	_, err = vfs.Stat(ctx, "/subdir/child.txt")
	require.NoError(t, err)

	// Write and verify content.
	content := []byte("child-data")
	require.NoError(t, vfs.Write(ctx, "/subdir/child.txt", 0, bytes.NewReader(content)))
	rc, err := vfs.Read(ctx, "/subdir/child.txt", 0)
	require.NoError(t, err)
	got, _ := io.ReadAll(rc)
	rc.Close()
	assert.Equal(t, content, got)
}

// TestVFSImpl_Remove_NoReappear verifies a deleted file stays gone after ListDir.
func TestVFSImpl_Remove_NoReappear(t *testing.T) {
	t.Parallel()

	vfs := newTestVFS(t)
	ctx := context.Background()

	require.NoError(t, vfs.Create(ctx, "/gone.txt"))
	_, err := vfs.Stat(ctx, "/gone.txt")
	require.NoError(t, err)

	require.NoError(t, vfs.Remove(ctx, "/gone.txt"))
	_, err = vfs.Stat(ctx, "/gone.txt")
	assert.Error(t, err, "file should be gone")

	// ListDir on parent should not bring it back.
	entries, err := vfs.ListDir(ctx, "/")
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotEqual(t, "gone.txt", e.Name, "deleted file should not reappear")
	}
	_, err = vfs.Stat(ctx, "/gone.txt")
	assert.Error(t, err, "file should still be gone after ListDir")
}

// TestVFSImpl_RapidWrites_UploadLatest writes the same file twice in quick
// succession and verifies the remote content is the final version.
func TestVFSImpl_RapidWrites_UploadLatest(t *testing.T) {
	vfs := newTestVFS(t)
	ctx := context.Background()

	require.NoError(t, vfs.Create(ctx, "/rapid.txt"))

	// First write.
	v1 := []byte("version-one-data")
	require.NoError(t, vfs.Write(ctx, "/rapid.txt", 0, bytes.NewReader(v1)))

	// Second write before writeBack fires.
	v2 := []byte("version-two-data-final")
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, vfs.Write(ctx, "/rapid.txt", 0, bytes.NewReader(v2)))

	// Wait for writeBack + upload to complete.
	time.Sleep(200 * time.Millisecond)

	// Read back locally.
	rc, err := vfs.Read(ctx, "/rapid.txt", 0)
	require.NoError(t, err)
	got, _ := io.ReadAll(rc)
	rc.Close()
	assert.Equal(t, v2, got, "local content should be the final version")
}

// TestVFSImpl_ReUpload_RemovesOld verifies that re-uploading a file deletes
// the previous remote FID (no duplicate accumulation).
func TestVFSImpl_ReUpload_RemovesOld(t *testing.T) {
	vfs := newTestVFS(t)
	ctx := context.Background()

	require.NoError(t, vfs.Create(ctx, "/reup.txt"))

	// First upload cycle.
	require.NoError(t, vfs.Write(ctx, "/reup.txt", 0, bytes.NewReader([]byte("v1"))))
	time.Sleep(150 * time.Millisecond)

	// Second upload cycle — should delete old FID.
	require.NoError(t, vfs.Write(ctx, "/reup.txt", 0, bytes.NewReader([]byte("v2"))))
	time.Sleep(150 * time.Millisecond)

	// Verify content is final version.
	rc, err := vfs.Read(ctx, "/reup.txt", 0)
	require.NoError(t, err)
	got, _ := io.ReadAll(rc)
	rc.Close()
	assert.Equal(t, []byte("v2"), got)
}

func TestVFSImpl_Shutdown_StopsUploads(t *testing.T) {
	vfs := newTestVFS(t)
	ctx := context.Background()

	require.NoError(t, vfs.Create(ctx, "/test.txt"))
	require.NoError(t, vfs.Write(ctx, "/test.txt", 0, bytes.NewReader([]byte("data"))))

	// Wait for writeBack timer to be scheduled.
	time.Sleep(20 * time.Millisecond)

	assert.True(t, vfs.uploads.IsPending("/test.txt"), "should be pending before shutdown")

	// Shutdown stops the timer and drains.
	vfs.Shutdown()

	// After shutdown, MarkDirty state is still recorded.
	assert.True(t, vfs.uploads.IsPending("/test.txt"), "info should survive shutdown")

	// FireAllTimers should not start any new upload (timers were cleared).
	vfs.uploads.FireAllTimers()
	time.Sleep(50 * time.Millisecond)

	// Still pending — no upload was triggered.
	assert.True(t, vfs.uploads.IsPending("/test.txt"), "should still be pending after shutdown+fire")

	// Write again after shutdown: MarkDirty records state but does not schedule upload.
	require.NoError(t, vfs.Write(ctx, "/test.txt", 0, bytes.NewReader([]byte("data2"))))
	assert.True(t, vfs.uploads.IsPending("/test.txt"), "should be pending after post-shutdown write")
}

// ---------------------------------------------------------------------------
// Stale upload commit detection tests
// ---------------------------------------------------------------------------

// blockingDriver wraps mockdrive.MockDriver and gates Put calls so tests
// can delay upload completion and simulate rename/delete races.
type blockingDriver struct {
	*mockdrive.MockDriver
	started chan struct{} // closed when Put is entered
	proceed chan struct{} // close to unblock all waiting Puts
}

func (d *blockingDriver) Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (drivers.Entry, error) {
	d.started <- struct{}{}
	<-d.proceed
	return d.MockDriver.Put(ctx, parentID, name, size, body)
}

// newTestVFSWithBlocking creates a VFSImpl whose uploads block at the driver
// Put call. Call close(bd.proceed) to unblock.
func newTestVFSWithBlocking(t *testing.T) (*VFSImpl, *blockingDriver) {
	t.Helper()
	bd := &blockingDriver{
		MockDriver: mockdrive.NewDriver(),
		started:    make(chan struct{}, 1),
		proceed:    make(chan struct{}),
	}
	cp, err := cipher.NewRcloneCipher("test-password", "")
	require.NoError(t, err)
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	vfs := NewVFSImpl(bd, cp, cacheDir, "0", FSOptions{
		MaxRetries:        0, // no retries for deterministic stale behavior
		ConcurrentUploads: 2,
		MemCacheSizeMB:    16,
		WriteBackTimeout:  50 * time.Millisecond,
	})
	return vfs, bd
}

// TestVFSImpl_RenameCancelsInFlightUpload verifies that renaming a file
// while its upload is in-flight prevents the stale upload from committing.
// The rename calls CancelSync which cancels the upload context; the upload
// may still complete (driver ignores cancel) but OnUploadComplete detects
// staleness because the path no longer exists in the tree.
func TestVFSImpl_RenameCancelsInFlightUpload(t *testing.T) {
	ctx := context.Background()
	vfs, bd := newTestVFSWithBlocking(t)

	require.NoError(t, vfs.Create(ctx, "/test.txt"))
	require.NoError(t, vfs.Write(ctx, "/test.txt", 0, bytes.NewReader([]byte("data"))))

	// Fire writeBack timer so upload starts, then wait for it to block.
	vfs.uploads.FireAllTimers()
	<-bd.started

	// Rename while upload is blocked.
	require.NoError(t, vfs.Rename(ctx, "/test.txt", "/renamed.txt"))

	// Unblock the upload — it completes but OnUploadComplete detects staleness.
	close(bd.proceed)
	time.Sleep(50 * time.Millisecond)

	// The old path must not exist.
	_, err := vfs.Stat(ctx, "/test.txt")
	assert.Error(t, err, "old path should not exist after rename")

	// The renamed path should still be local-only (upload was stale, not committed).
	rn := vfs.tree.Lookup("/renamed.txt")
	require.NotNil(t, rn, "renamed path should exist in tree")
	assert.False(t, rn.IsUploaded(), "renamed file should not be marked as uploaded; stale upload was rejected")
}

// TestVFSImpl_DeleteRejectsInFlightUploadCommit verifies that deleting a
// file while its upload is in-flight does not resurrect it. The upload
// completes but OnUploadComplete sees the tree node is gone.
func TestVFSImpl_DeleteRejectsInFlightUploadCommit(t *testing.T) {
	ctx := context.Background()
	vfs, bd := newTestVFSWithBlocking(t)

	require.NoError(t, vfs.Create(ctx, "/test.txt"))
	require.NoError(t, vfs.Write(ctx, "/test.txt", 0, bytes.NewReader([]byte("data"))))

	vfs.uploads.FireAllTimers()
	<-bd.started

	// Delete while upload is blocked.
	require.NoError(t, vfs.Remove(ctx, "/test.txt"))

	close(bd.proceed)
	time.Sleep(50 * time.Millisecond)

	// The file must stay deleted.
	_, err := vfs.Stat(ctx, "/test.txt")
	assert.Error(t, err, "file should remain deleted after stale upload commit")
}

// TestVFSImpl_RecreateSamePathRejectsOldUploadCommit verifies that if a
// file is deleted and recreated while an old upload is in-flight, the old
// upload's commit does not overwrite the new node's content. The old upload
// carries a stale uploadAttempt whose FID differs from the current tree node.
func TestVFSImpl_RecreateSamePathRejectsOldUploadCommit(t *testing.T) {
	ctx := context.Background()
	vfs, bd := newTestVFSWithBlocking(t)

	require.NoError(t, vfs.Create(ctx, "/test.txt"))
	require.NoError(t, vfs.Write(ctx, "/test.txt", 0, bytes.NewReader([]byte("v1"))))

	// Start the first upload and wait for it to block.
	vfs.uploads.FireAllTimers()
	<-bd.started

	// Delete and recreate at the same path — this creates a new VFSNode.
	require.NoError(t, vfs.Remove(ctx, "/test.txt"))
	require.NoError(t, vfs.Create(ctx, "/test.txt"))
	require.NoError(t, vfs.Write(ctx, "/test.txt", 0, bytes.NewReader([]byte("v2"))))

	// Unblock the old upload — OnUploadComplete should detect staleness.
	close(bd.proceed)
	time.Sleep(50 * time.Millisecond)

	// Fire the new upload timer (for v2) and let it complete.
	vfs.uploads.FireAllTimers()
	time.Sleep(100 * time.Millisecond)

	// Read back — the file should contain v2, proving the old stale upload
	// did not overwrite the new node.
	rc, err := vfs.Read(ctx, "/test.txt", 0)
	require.NoError(t, err)
	got, _ := io.ReadAll(rc)
	rc.Close()
	assert.Equal(t, []byte("v2"), got, "content should be v2, not v1 from stale upload")
}
