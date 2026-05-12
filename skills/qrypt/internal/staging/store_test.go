package staging

import (
	"os"
	"path/filepath"
	"testing"
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
	if !filepath.HasPrefix(path, dir) {
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
