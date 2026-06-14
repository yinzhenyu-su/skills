//go:build integration

package vfs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/drivers/factory"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

// setupE2E creates a VFS backed by the first test-enabled mount from config.
// It returns the VFS, the driver (for raw cleanup), the test root path,
// and a cleanup function.
func setupE2E(t *testing.T) (*VFSImpl, drivers.Writer, string, func()) {
	t.Helper()
	cfgPath := os.Getenv("QRYPT_CONFIG")
	if cfgPath == "" {
		cfgPath = findConfig()
	}
	if cfgPath == "" {
		t.Skip("no qrypt config found")
	}

	cfg, _, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	mounts := cfg.DiscoverTestMounts()
	if len(mounts) == 0 {
		t.Fatal("no test-enabled mounts found")
	}
	m := mounts[0]
	rcCfg := cfg.MergeInstanceConfig(m)

	cp, err := config.MakeCipher(rcCfg.Encryption, cfg.Defaults.Encryption, "", "")
	if err != nil {
		t.Fatalf("make cipher: %v", err)
	}

	drv, err := factory.NewDriverFromType(rcCfg.Type, rcCfg.Params)
	if err != nil {
		t.Fatalf("create driver: %v", err)
	}
	if err := drv.Init(context.Background()); err != nil {
		t.Fatalf("init driver: %v", err)
	}

	writer, _ := drv.(drivers.Writer)

	cacheDir := t.TempDir()
	vfs := NewVFSImpl(drv, cp, cacheDir, "0", FSOptions{
		MaxRetries:        2,
		ConcurrentUploads: 2,
		MemCacheSizeMB:    16,
		WriteBackTimeout:  3 * time.Second,
	})

	testRoot := fmt.Sprintf("/qrypt-e2e-%d", time.Now().UnixNano())
	ctx := context.Background()
	if err := vfs.Mkdir(ctx, testRoot); err != nil {
		t.Fatalf("mkdir test root: %v", err)
	}

	cleanup := func() {
		vfs.Shutdown()
		drv.Drop(context.Background())
	}

	return vfs, writer, testRoot, cleanup
}

func TestE2E_QuarkDrive(t *testing.T) {
	vfs, drv, testRoot, cleanup := setupE2E(t)
	defer cleanup()

	ctx := context.Background()
	filePath := testRoot + "/hello.txt"
	t.Logf("creating %s", filePath)
	if err := vfs.Create(ctx, filePath); err != nil {
		t.Fatalf("create: %v", err)
	}

	content := []byte("hello e2e from vfs " + time.Now().Format(time.RFC3339))
	if err := vfs.Write(ctx, filePath, 0, bytes.NewReader(content)); err != nil {
		t.Fatalf("write: %v", err)
	}

	fr, err := vfs.Read(ctx, filePath, 0)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got, _ := io.ReadAll(fr)
	fr.Close()
	if string(got) != string(content) {
		t.Fatalf("content mismatch: got %q, want %q", got, content)
	}
	t.Logf("local read OK: %s", content)

	t.Log("waiting for upload...")
	time.Sleep(8 * time.Second)

	entries, err := vfs.ListDir(ctx, testRoot)
	if err != nil {
		t.Fatalf("list dir: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Name == "hello.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("hello.txt not found in directory listing after upload")
	}
	t.Logf("remote listing OK: found hello.txt")

	info, err := vfs.Stat(ctx, filePath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size != int64(len(content)) {
		t.Fatalf("size mismatch: got %d, want %d", info.Size, len(content))
	}
	t.Logf("stat OK: size=%d", info.Size)

	v1 := []byte("v1-first-version")
	v2 := []byte("v2-final-version")
	if err := vfs.Write(ctx, filePath, 0, bytes.NewReader(v1)); err != nil {
		t.Fatalf("write v1: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := vfs.Write(ctx, filePath, 0, bytes.NewReader(v2)); err != nil {
		t.Fatalf("write v2: %v", err)
	}

	fr2, err := vfs.Read(ctx, filePath, 0)
	if err != nil {
		t.Fatalf("read after rapid write: %v", err)
	}
	got2, _ := io.ReadAll(fr2)
	fr2.Close()
	if string(got2) != string(v2) {
		t.Fatalf("rapid write: got %q, want %q", got2, v2)
	}
	t.Log("rapid write OK: final version is v2")

	t.Log("waiting for second upload...")
	time.Sleep(8 * time.Second)
	entries2, err := vfs.ListDir(ctx, testRoot)
	if err != nil {
		t.Fatalf("list dir after re-upload: %v", err)
	}
	if len(entries2) != 1 {
		t.Fatalf("expected 1 file after re-upload, got %d files (duplicate detected)", len(entries2))
	}
	t.Log("no duplicate: exactly 1 file after re-upload")

	// Clean up via VFS.
	if err := vfs.Remove(ctx, filePath); err == nil {
		time.Sleep(100 * time.Millisecond)
	}
	if err := vfs.Remove(ctx, testRoot); err == nil {
		time.Sleep(100 * time.Millisecond)
	}
	if info, err := vfs.Stat(ctx, testRoot); err == nil {
		_ = drv.Remove(ctx, drivers.Entry{ID: info.ID})
	}
}

// TestE2E_SubdirFileNoPhantomRoot verifies that a file created inside a
// subdirectory does NOT appear at the parent (or root) level after upload.
// This is a regression test for the OnUploadComplete bug where the node was
// stored at "/"+name instead of its actual path.
func TestE2E_SubdirFileNoPhantomRoot(t *testing.T) {
	vfs, drv, testRoot, cleanup := setupE2E(t)
	defer cleanup()

	ctx := context.Background()
	subDir := testRoot + "/sub"
	filePath := subDir + "/index.html"

	// Create subdir + file
	if err := vfs.Mkdir(ctx, subDir); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	if err := vfs.Create(ctx, filePath); err != nil {
		t.Fatalf("create index.html: %v", err)
	}
	content := []byte("subfile content")
	if err := vfs.Write(ctx, filePath, 0, bytes.NewReader(content)); err != nil {
		t.Fatalf("write index.html: %v", err)
	}

	// Verify local tree: file exists only in sub/, not at testRoot
	rootEntries, err := vfs.ListDir(ctx, testRoot)
	if err != nil {
		t.Fatalf("list testRoot: %v", err)
	}
	for _, e := range rootEntries {
		if e.Name == "index.html" {
			t.Fatal("index.html found at root BEFORE upload — phantom detected")
		}
	}
	t.Log("local: no phantom index.html at root (before upload)")

	// Wait for upload
	t.Log("waiting for upload...")
	time.Sleep(8 * time.Second)

	// Verify after upload: still no phantom at root
	rootEntries, err = vfs.ListDir(ctx, testRoot)
	if err != nil {
		t.Fatalf("list testRoot after upload: %v", err)
	}
	for _, e := range rootEntries {
		if e.Name == "index.html" {
			t.Fatal("index.html found at root AFTER upload — phantom detected")
		}
	}
	t.Log("AFTER upload: no phantom index.html at root")

	// Verify file exists in subdir
	subEntries, err := vfs.ListDir(ctx, subDir)
	if err != nil {
		t.Fatalf("list subDir: %v", err)
	}
	found := false
	for _, e := range subEntries {
		if e.Name == "index.html" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("index.html not found in subdir")
	}
	t.Log("file correctly present in subdir")

	// Cleanup
	if err := vfs.Remove(ctx, filePath); err == nil {
		time.Sleep(100 * time.Millisecond)
	}
	if err := vfs.Remove(ctx, subDir); err == nil {
		time.Sleep(100 * time.Millisecond)
	}
	if err := vfs.Remove(ctx, testRoot); err == nil {
		time.Sleep(100 * time.Millisecond)
	}
	if info, err := vfs.Stat(ctx, testRoot); err == nil {
		_ = drv.Remove(ctx, drivers.Entry{ID: info.ID})
	}
}

// TestE2E_DeleteDirNoReappear verifies that after deleting a directory and
// waiting for remote refresh, the deleted directory does NOT reappear in the
// listing. This is a regression test for the race between cleanupLocal
// (removing the deleting guard) and a subsequent remote refresh.
func TestE2E_DeleteDirNoReappear(t *testing.T) {
	vfs, drv, testRoot, cleanup := setupE2E(t)
	defer cleanup()

	ctx := context.Background()
	subDir := testRoot + "/todelete"
	filePath := subDir + "/file.txt"

	// Create directory with a file
	if err := vfs.Mkdir(ctx, subDir); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := vfs.Create(ctx, filePath); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := vfs.Write(ctx, filePath, 0, bytes.NewReader([]byte("delete me"))); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Wait for upload to complete
	t.Log("waiting for upload...")
	time.Sleep(10 * time.Second)

	// Verify it exists before deletion
	entries, err := vfs.ListDir(ctx, testRoot)
	if err != nil {
		t.Fatalf("list before delete: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Name == "todelete" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("todelete not found before deletion")
	}
	t.Log("directory present before delete")

	// Delete the directory
	if err := vfs.Remove(ctx, filePath); err != nil {
		t.Fatalf("remove file: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := vfs.Remove(ctx, subDir); err != nil {
		t.Fatalf("remove dir: %v", err)
	}

	// Verify gone immediately
	entries, err = vfs.ListDir(ctx, testRoot)
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	for _, e := range entries {
		if e.Name == "todelete" {
			t.Fatal("todelete still present immediately after delete")
		}
	}
	t.Log("gone immediately after delete")

	// Wait long enough for:
	// 1. Coalescer flush → executeDelete → BatchRemove
	// 2. Remote delete API propagation
	// 3. Remote refresh from ListDir calls
	t.Log("waiting for remote refresh cycle...")
	for i := 0; i < 6; i++ {
		time.Sleep(5 * time.Second)
		entries, err = vfs.ListDir(ctx, testRoot)
		if err != nil {
			t.Fatalf("list during refresh wait: %v", err)
		}
		for _, e := range entries {
			if e.Name == "todelete" {
				t.Fatalf("todelete REAPPEARED %ds after delete", (i+1)*5)
			}
		}
	}
	t.Log("directory did NOT reappear after 30s of remote refresh cycles")

	// Cleanup: remove testRoot via VFS
	if err := vfs.Remove(ctx, testRoot); err == nil {
		time.Sleep(100 * time.Millisecond)
	}
	if info, err := vfs.Stat(ctx, testRoot); err == nil {
		_ = drv.Remove(ctx, drivers.Entry{ID: info.ID})
	}
}

func findConfig() string {
	workDir := os.Getenv("QRYPT_WORK_DIR")
	if workDir == "" {
		workDir = filepath.Join(os.Getenv("HOME"), ".qrypt")
	}
	p := filepath.Join(workDir, "qrypt.toml")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

