package upload

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewStore(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Dir() != dir {
		t.Errorf("expected %s, got %s", dir, s.Dir())
	}
}

func TestCreateAndPath(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)

	path, err := s.Create("test_fid_123")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %s", path)
	}
	if !strings.HasPrefix(path, dir) {
		t.Errorf("expected under %s, got %s", dir, path)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 0 {
		t.Errorf("expected empty file, got %d", info.Size())
	}
}

func TestWriteAndReadAt(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)

	path, _ := s.Create("fid_write_test")

	n, err := s.WriteAt(path, []byte("hello world"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 11 {
		t.Errorf("expected 11 bytes written, got %d", n)
	}

	buf := make([]byte, 5)
	n, err = s.ReadAt(path, buf, 6)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "world" {
		t.Errorf("expected 'world', got '%s'", string(buf[:n]))
	}
}

func TestWriteAt_Offset(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	path, _ := s.Create("fid_offset")

	s.WriteAt(path, []byte("aaaaa"), 0)
	s.WriteAt(path, []byte("BBB"), 2)

	buf := make([]byte, 5)
	n, err := s.ReadAt(path, buf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "aaBBB" {
		t.Errorf("expected 'aaBBB', got '%s'", string(buf[:n]))
	}
}

func TestOpenReader(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	path, _ := s.Create("fid_reader")
	s.WriteAt(path, []byte("test data"), 0)

	r, err := s.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	buf := make([]byte, 9)
	n, _ := r.Read(buf)
	if string(buf[:n]) != "test data" {
		t.Errorf("expected 'test data', got '%s'", string(buf[:n]))
	}
}

func TestExists(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	path, _ := s.Create("fid_exists")

	if !s.Exists(path) {
		t.Error("expected file to exist")
	}
	if s.Exists("/nonexistent/staging/file") {
		t.Error("expected false for nonexistent")
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	path, _ := s.Create("fid_remove")

	if !s.Exists(path) {
		t.Fatal("expected file to exist")
	}
	s.Remove(path)
	if s.Exists(path) {
		t.Error("expected file to be removed")
	}
}

func TestRemove_EmptyPath(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	if err := s.Remove(""); err != nil {
		t.Errorf("expected no error for empty path, got %v", err)
	}
}

func TestFileSize(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	path, _ := s.Create("fid_size")
	s.WriteAt(path, []byte("1234567890"), 0)

	size, err := s.FileSize(path)
	if err != nil {
		t.Fatal(err)
	}
	if size != 10 {
		t.Errorf("expected 10, got %d", size)
	}
}

func TestTruncate(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	path, _ := s.Create("fid_trunc")
	s.WriteAt(path, []byte("hello world truncate"), 0)

	if err := s.Truncate(path, 5); err != nil {
		t.Fatal(err)
	}

	size, _ := s.FileSize(path)
	if size != 5 {
		t.Errorf("expected 5, got %d", size)
	}
}

func TestSync(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	path, _ := s.Create("fid_sync")
	s.WriteAt(path, []byte("sync test"), 0)

	if err := s.Sync(path); err != nil {
		t.Fatal(err)
	}
}

func TestListStagingFiles(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	s.Create("fid_a")
	s.Create("fid_b")

	files, err := s.ListStagingFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
}

func TestCleanupOrphanedStagingFiles(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	s.Create("fid_active")
	s.Create("fid_orphan")

	active := map[string]bool{"fid_active": true}
	cleaned, err := s.CleanupOrphanedStagingFiles(active)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleaned) != 1 {
		t.Errorf("expected 1 orphan cleaned, got %d", len(cleaned))
	}
}

func TestFidFromPath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/cache/staging/fid123.staging", "fid123"},
		{"/cache/staging/normal", "normal"},
	}
	for _, tt := range tests {
		result := FidFromPath(tt.path)
		if result != tt.expected {
			t.Errorf("FidFromPath(%q) = %q, want %q", tt.path, result, tt.expected)
		}
	}
}

func TestEnsure(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	newPath := filepath.Join(dir, "new_file.staging")

	if s.Exists(newPath) {
		t.Fatal("should not exist yet")
	}
	if err := s.Ensure(newPath); err != nil {
		t.Fatal(err)
	}
	if !s.Exists(newPath) {
		t.Error("should exist after Ensure")
	}
}

// ============================================================
// Writeback page buffer tests
// ============================================================

func TestWriteback_ConsecutiveWritesCoalesce(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	path, _ := s.Create("fid_coalesce")

	// Multiple small writes — should all go to the page buffer.
	s.WriteAt(path, []byte("AAA"), 0)
	s.WriteAt(path, []byte("BBB"), 3)
	s.WriteAt(path, []byte("CCC"), 6)

	// ReadAt should return merged data from the page buffer.
	buf := make([]byte, 9)
	n, err := s.ReadAt(path, buf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "AAABBBCCC" {
		t.Errorf("got %q, want %q", string(buf[:n]), "AAABBBCCC")
	}
}

func TestWriteback_FlushOnSync(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	path, _ := s.Create("fid_syncflush")

	s.WriteAt(path, []byte("sync flush data"), 0)

	// Sync flushes page + fsyncs disk.
	if err := s.Sync(path); err != nil {
		t.Fatal(err)
	}

	// After Sync, the page should be flushed and disk file complete.
	buf := make([]byte, 15)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	n, err := f.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "sync flush data" {
		t.Errorf("got %q, want %q", string(buf[:n]), "sync flush data")
	}
}

func TestWriteback_ReadFromPageVsDisk(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	path, _ := s.Create("fid_readvsdisk")

	// Write only to page buffer (no flush).
	s.WriteAt(path, []byte("page data"), 0)

	// ReadAt should serve from page buffer without touching disk.
	buf := make([]byte, 9)
	n, err := s.ReadAt(path, buf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "page data" {
		t.Errorf("got %q, want %q", string(buf[:n]), "page data")
	}

	// Disk file should be empty (page not flushed yet).
	finfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if finfo.Size() != 0 {
		t.Errorf("expected empty disk file before flush, got size %d", finfo.Size())
	}
}

func TestWriteback_ReadBeyondPageFlushesToDisk(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	path, _ := s.Create("fid_beyondpage")

	// Write 5 bytes to page buffer.
	s.WriteAt(path, []byte("hello"), 0)

	// Read 5 bytes (within page range) — should serve from page buffer.
	buf := make([]byte, 5)
	n, err := s.ReadAt(path, buf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 || string(buf[:n]) != "hello" {
		t.Errorf("got %q (len=%d), want %q", string(buf[:n]), n, "hello")
	}

	// Read beyond the buffered range (offset 10) — should flush page first,
	// then read from disk (which may return short/EOF since file is small).
	bigBuf := make([]byte, 20)
	n, err = s.ReadAt(path, bigBuf, 0)
	if err != nil && n == 0 {
		t.Fatal(err)
	}
	_ = n

	// After the flush, disk should have the data.
	finfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if finfo.Size() != 5 {
		t.Errorf("expected disk size 5 after flush, got %d", finfo.Size())
	}
}

func TestWriteback_CloseFlushesPage(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	path, _ := s.Create("fid_closeflush")

	s.WriteAt(path, []byte("close flush data"), 0)

	// Close should flush the page to disk.
	if err := s.Close(path); err != nil {
		t.Fatal(err)
	}

	// Disk file should now have the data.
	buf := make([]byte, 16)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	n, err := f.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "close flush data" {
		t.Errorf("got %q, want %q", string(buf[:n]), "close flush data")
	}
}

func TestWriteback_CloseIdempotent(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	path, _ := s.Create("fid_idemclose")

	s.WriteAt(path, []byte("data"), 0)
	if err := s.Close(path); err != nil {
		t.Fatal(err)
	}
	// Second close should be a no-op (not panic, not error).
	if err := s.Close(path); err != nil {
		t.Fatal(err)
	}
}

func TestWriteback_PageFullFlushesEarly(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	path, _ := s.Create("fid_pagefull")

	// Use writes < pageMaxSize/4 (256KB) so they route through the page buffer.
	// 6 × 200KB = 1.2MB > pageMaxSize (1,048,576) → triggers early flush on 6th write.
	chunk := bytes.Repeat([]byte("A"), 200*1024) // 200KB
	for i := 0; i < 5; i++ {
		s.WriteAt(path, chunk, int64(i*len(chunk)))
	}

	// After 5 writes: 1MB, still under pageMaxSize (1,048,576).
	// Disk should be empty (all in page buffer).
	finfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if finfo.Size() != 0 {
		t.Fatalf("expected empty disk before flush, got %d", finfo.Size())
	}

	// 6th write: 1.2MB total → exceeds pageMaxSize → triggers early flush.
	s.WriteAt(path, chunk, int64(5*len(chunk)))

	time.Sleep(100 * time.Millisecond)

	finfo, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if finfo.Size() == 0 {
		t.Error("expected non-empty disk after page-full flush, but file is still empty")
	}
}

func TestWriteback_LargeWriteFallthrough(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	path, _ := s.Create("fid_large")

	// A write larger than pageMaxSize/4 should go directly to disk.
	largeData := bytes.Repeat([]byte("X"), pageMaxSize/2)
	s.WriteAt(path, largeData, 0)

	// Disk should have the data immediately (no page buffer).
	finfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if finfo.Size() == 0 {
		t.Error("expected large write to land on disk immediately")
	}
}

func TestWriteback_ConcurrentWritesSameFid(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	path, _ := s.Create("fid_concurrent")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			off := int64(idx * 3)
			data := []byte{byte('A' + idx), byte('A' + idx), byte('A' + idx)}
			s.WriteAt(path, data, off)
		}(i)
	}
	wg.Wait()

	// Sync to flush all buffered writes.
	if err := s.Sync(path); err != nil {
		t.Fatal(err)
	}

	// Read back and verify all 10 writes are present.
	buf := make([]byte, 30)
	n, err := s.ReadAt(path, buf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n != 30 {
		t.Fatalf("expected 30 bytes, got %d", n)
	}
	expected := ""
	for i := 0; i < 10; i++ {
		expected += string([]byte{byte('A' + i), byte('A' + i), byte('A' + i)})
	}
	if string(buf[:n]) != expected {
		t.Errorf("concurrent writes: got %q, want %q", string(buf[:n]), expected)
	}
}

func TestWriteback_ConcurrentWriteAndRead(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	path, _ := s.Create("fid_conc_rw")

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer goroutine.
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			off := int64(i * 2)
			s.WriteAt(path, []byte("AB"), off)
		}
	}()

	// Reader goroutine.
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			buf := make([]byte, 4)
			s.ReadAt(path, buf, 0)
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()

	// Final read should succeed.
	if err := s.Sync(path); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 100)
	n, err := s.ReadAt(path, buf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Error("expected data after concurrent write/read")
	}
}

func TestWriteback_TruncateInvalidatesPage(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	path, _ := s.Create("fid_truncpage")

	s.WriteAt(path, []byte("hello world truncate"), 0)

	// Truncate should flush the page first, then truncate, then drop the page.
	if err := s.Truncate(path, 5); err != nil {
		t.Fatal(err)
	}

	// Read should return only 5 bytes.
	buf := make([]byte, 5)
	n, err := s.ReadAt(path, buf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "hello" {
		t.Errorf("got %q, want %q", string(buf[:n]), "hello")
	}
}

func TestWriteback_WriteAfterTruncate(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStore(dir)
	path, _ := s.Create("fid_writetrunc")

	s.WriteAt(path, []byte("before truncate"), 0)
	s.Truncate(path, 5)
	s.WriteAt(path, []byte("HELLO"), 0)

	buf := make([]byte, 5)
	n, err := s.ReadAt(path, buf, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "HELLO" {
		t.Errorf("got %q, want %q", string(buf[:n]), "HELLO")
	}
}
