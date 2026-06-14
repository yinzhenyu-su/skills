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

func TestE2E_QuarkDrive(t *testing.T) {
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
	var cleanupPaths []string
	cleanup := func() {
		for _, p := range cleanupPaths {
			if writer != nil {
				_ = writer.Remove(context.Background(), drivers.Entry{ID: p})
			}
		}
		drv.Drop(context.Background())
	}
	defer cleanup()

	cacheDir := t.TempDir()
	vfs := NewVFSImpl(drv, cp, cacheDir, "0", FSOptions{
		MaxRetries:        2,
		ConcurrentUploads: 2,
		MemCacheSizeMB:    16,
		WriteBackTimeout:  3 * time.Second,
	})
	defer vfs.Shutdown()

	ctx := context.Background()
	testRoot := fmt.Sprintf("/qrypt-e2e-%d", time.Now().UnixNano())

	t.Logf("creating test directory %s", testRoot)
	if err := vfs.Mkdir(ctx, testRoot); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

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

	// Wait for the second upload to complete and verify no duplicate.
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

	// Clean up via VFS first (for local tree consistency).
	if err := vfs.Remove(ctx, filePath); err == nil {
		time.Sleep(100 * time.Millisecond)
	}
	if err := vfs.Remove(ctx, testRoot); err == nil {
		time.Sleep(100 * time.Millisecond)
	}

	// Deferred cleanup will handle any remaining remote files via raw driver.
	// Register the test directory fid so cleanup can find and remove it.
	if info, err := vfs.Stat(ctx, testRoot); err == nil {
		cleanupPaths = append(cleanupPaths, info.ID)
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

