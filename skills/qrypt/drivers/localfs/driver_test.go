package localfs

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yinzhenyu/skills/qrypt/drivers"
)

func newTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return dir
}

func TestNewDriver(t *testing.T) {
	dir := newTestDir(t)
	d := NewDriver(dir)
	if d == nil {
		t.Fatal("expected non-nil driver")
	}
}

func TestInit_Success(t *testing.T) {
	dir := newTestDir(t)
	d := NewDriver(dir)
	err := d.Init(context.Background())
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
}

func TestInit_NonExistent(t *testing.T) {
	d := NewDriver("/nonexistent_path_xyz")
	err := d.Init(context.Background())
	if err == nil {
		t.Fatal("expected error for nonexistent root")
	}
}

func TestInit_NotADirectory(t *testing.T) {
	dir := newTestDir(t)
	filePath := filepath.Join(dir, "file.txt")
	os.WriteFile(filePath, []byte("data"), 0o644)

	d := NewDriver(filePath)
	err := d.Init(context.Background())
	if err == nil {
		t.Fatal("expected error when root is a file")
	}
}

func TestDrop(t *testing.T) {
	dir := newTestDir(t)
	d := NewDriver(dir)
	d.Init(context.Background())
	err := d.Drop(context.Background())
	if err != nil {
		t.Fatalf("Drop failed: %v", err)
	}
}

func TestList_EmptyDir(t *testing.T) {
	dir := newTestDir(t)
	d := NewDriver(dir)
	d.Init(context.Background())

	entries, err := d.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestList_WithFiles(t *testing.T) {
	dir := newTestDir(t)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb"), 0o644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)

	d := NewDriver(dir)
	d.Init(context.Background())

	entries, err := d.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	if !names["a.txt"] {
		t.Error("expected a.txt in listing")
	}
	if !names["b.txt"] {
		t.Error("expected b.txt in listing")
	}
	if !names["sub"] {
		t.Error("expected sub in listing")
	}
}

func TestRead_Success(t *testing.T) {
	dir := newTestDir(t)
	path := filepath.Join(dir, "data.bin")
	content := []byte("hello localfs driver")
	os.WriteFile(path, content, 0o644)

	d := NewDriver(dir)
	d.Init(context.Background())

	entries, _ := d.List(context.Background(), "")
	if len(entries) == 0 {
		t.Fatal("expected at least one entry")
	}

	entry := entries[0]
	reader, err := d.Read(context.Background(), entry, 0, int64(len(content)))
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	defer reader.Close()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("read content mismatch: got %q, want %q", string(got), string(content))
	}
}

func TestRead_Offset(t *testing.T) {
	dir := newTestDir(t)
	path := filepath.Join(dir, "offset.bin")
	content := []byte("0123456789")
	os.WriteFile(path, content, 0o644)

	d := NewDriver(dir)
	d.Init(context.Background())

	entries, _ := d.List(context.Background(), "")
	if len(entries) == 0 {
		t.Fatal("expected at least one entry")
	}

	reader, err := d.Read(context.Background(), entries[0], 3, 4)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	defer reader.Close()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(got) != "3456" {
		t.Errorf("expected '3456', got %q", string(got))
	}
}

func TestRead_NonExistent(t *testing.T) {
	dir := newTestDir(t)
	d := NewDriver(dir)
	d.Init(context.Background())

	_, err := d.Read(context.Background(), drivers.Entry{ID: filepath.Join(dir, "nonexistent")}, 0, 10)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestMkdir(t *testing.T) {
	dir := newTestDir(t)
	d := NewDriver(dir)
	d.Init(context.Background())

	entry, err := d.Mkdir(context.Background(), "", "newdir")
	if err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}
	if !entry.IsDir {
		t.Error("expected directory entry")
	}
	if entry.Name != "newdir" {
		t.Errorf("expected name 'newdir', got %q", entry.Name)
	}

	// Verify on disk
	if _, err := os.Stat(filepath.Join(dir, "newdir")); os.IsNotExist(err) {
		t.Error("directory was not created on disk")
	}
}

func TestMkdir_Duplicate(t *testing.T) {
	dir := newTestDir(t)
	d := NewDriver(dir)
	d.Init(context.Background())

	d.Mkdir(context.Background(), "", "dupdir")
	_, err := d.Mkdir(context.Background(), "", "dupdir")
	if err == nil {
		t.Fatal("expected error for duplicate mkdir")
	}
}

func TestRename(t *testing.T) {
	dir := newTestDir(t)
	path := filepath.Join(dir, "old.txt")
	os.WriteFile(path, []byte("rename test"), 0o644)

	d := NewDriver(dir)
	d.Init(context.Background())

	entries, _ := d.List(context.Background(), "")
	if len(entries) == 0 {
		t.Fatal("expected entries")
	}

	err := d.Rename(context.Background(), entries[0], "new.txt")
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "old.txt")); !os.IsNotExist(err) {
		t.Error("old file should not exist after rename")
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); os.IsNotExist(err) {
		t.Error("new file should exist after rename")
	}
}

func TestRemove_File(t *testing.T) {
	dir := newTestDir(t)
	path := filepath.Join(dir, "remove_me.txt")
	os.WriteFile(path, []byte("bye"), 0o644)

	d := NewDriver(dir)
	d.Init(context.Background())

	entries, _ := d.List(context.Background(), "")
	if len(entries) == 0 {
		t.Fatal("expected entries")
	}

	err := d.Remove(context.Background(), entries[0])
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

func TestRemove_Directory(t *testing.T) {
	dir := newTestDir(t)
	subDir := filepath.Join(dir, "subdir")
	os.MkdirAll(subDir, 0o755)
	os.WriteFile(filepath.Join(subDir, "nested.txt"), []byte("nested"), 0o644)

	d := NewDriver(dir)
	d.Init(context.Background())

	entries, _ := d.List(context.Background(), "")
	if len(entries) == 0 {
		t.Fatal("expected entries")
	}

	err := d.Remove(context.Background(), entries[0])
	if err != nil {
		t.Fatalf("Remove dir failed: %v", err)
	}
	if _, err := os.Stat(subDir); !os.IsNotExist(err) {
		t.Error("directory should be deleted recursively")
	}
}

func TestMove(t *testing.T) {
	dir := newTestDir(t)
	srcDir := filepath.Join(dir, "src")
	dstDir := filepath.Join(dir, "dst")
	os.MkdirAll(srcDir, 0o755)
	os.MkdirAll(dstDir, 0o755)
	os.WriteFile(filepath.Join(srcDir, "move_me.txt"), []byte("moving"), 0o644)

	d := NewDriver(dir)
	d.Init(context.Background())

	// List src dir to get the file entry
	srcEntries, err := d.List(context.Background(), srcDir)
	if err != nil {
		t.Fatalf("List src failed: %v", err)
	}
	if len(srcEntries) == 0 {
		t.Fatal("expected entry in src")
	}

	err = d.Move(context.Background(), srcEntries[0], dstDir)
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(srcDir, "move_me.txt")); !os.IsNotExist(err) {
		t.Error("file should not exist in src after move")
	}
	if _, err := os.Stat(filepath.Join(dstDir, "move_me.txt")); os.IsNotExist(err) {
		t.Error("file should exist in dst after move")
	}
}

func TestPut(t *testing.T) {
	dir := newTestDir(t)
	d := NewDriver(dir)
	d.Init(context.Background())

	body := strings.NewReader("uploaded content")
	entry, err := d.Put(context.Background(), "", "uploaded.txt", 16, body)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if entry.Name != "uploaded.txt" {
		t.Errorf("expected name 'uploaded.txt', got %q", entry.Name)
	}

	content, err := os.ReadFile(filepath.Join(dir, "uploaded.txt"))
	if err != nil {
		t.Fatalf("reading uploaded file: %v", err)
	}
	if string(content) != "uploaded content" {
		t.Errorf("expected 'uploaded content', got %q", string(content))
	}
}

func TestResolvePath(t *testing.T) {
	dir := newTestDir(t)
	d := NewDriver(dir)
	d.Init(context.Background())

	// Valid paths
	abs, err := d.ResolvePath(context.Background(), "sub/file.txt")
	if err != nil {
		t.Fatalf("ResolvePath failed: %v", err)
	}
	if !strings.HasSuffix(abs, "sub/file.txt") {
		t.Errorf("unexpected resolved path: %s", abs)
	}

	// Path traversal prevention
	_, err = d.ResolvePath(context.Background(), "../outside")
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestLocalDriver_ImplementsInterfaces(t *testing.T) {
	// Compile-time interface checks (redundant with source, but explicit in tests)
	var d *LocalDriver
	var _ drivers.Driver = d
	var _ drivers.Writer = d
	var _ drivers.Uploader = d
}
