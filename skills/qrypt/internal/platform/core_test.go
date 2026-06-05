package platform

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

func TestNewFileAPIFromConfigForMount(t *testing.T) {
	dir := t.TempDir()
	localRoot1 := filepath.Join(dir, "mount1")
	localRoot2 := filepath.Join(dir, "mount2")
	if err := os.MkdirAll(localRoot1, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(localRoot2, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Version: "1",
		Mounts: []config.MountInstance{
			{
				Name:       "mount1",
				Type:       "localfs",
				MountPoint: filepath.Join(dir, "mp1"),
				Default:    true,
				Params: config.MountParams{
					"local_root": localRoot1,
				},
				Encryption: &config.EncryptionConfig{
					Password: "testpass1",
					Salt:     "testsalt1",
				},
			},
			{
				Name:       "mount2",
				Type:       "localfs",
				MountPoint: filepath.Join(dir, "mp2"),
				Default:    false,
				Params: config.MountParams{
					"local_root": localRoot2,
				},
				Encryption: &config.EncryptionConfig{
					Password: "testpass2",
					Salt:     "testsalt2",
				},
			},
		},
		Defaults: config.DefaultsConfig{
			Encryption: config.EncryptionConfig{
				Password: "defaultpass",
				Salt:     "defaultsalt",
			},
		},
	}

	api1, err := NewFileAPIFromConfigForMount(cfg, "mount1", "", "")
	if err != nil {
		t.Fatalf("failed to create FileAPI for mount1: %v", err)
	}
	if api1 == nil {
		t.Fatal("expected FileAPI for mount1, got nil")
	}

	api2, err := NewFileAPIFromConfigForMount(cfg, "mount2", "", "")
	if err != nil {
		t.Fatalf("failed to create FileAPI for mount2: %v", err)
	}
	if api2 == nil {
		t.Fatal("expected FileAPI for mount2, got nil")
	}
}

func TestNewFileAPIFromConfigForMount_NotFound(t *testing.T) {
	cfg := &config.Config{
		Version: "1",
		Mounts: []config.MountInstance{
			{
				Name:    "existing",
				Type:    "localfs",
				Default: true,
				Params: config.MountParams{
					"local_root": t.TempDir(),
				},
				Encryption: &config.EncryptionConfig{
					Password: "testpass",
					Salt:     "testsalt",
				},
			},
		},
	}

	_, err := NewFileAPIFromConfigForMount(cfg, "nonexistent", "", "")
	if err == nil {
		t.Fatal("expected error for nonexistent mount, got nil")
	}
}

func TestNewFileAPIFromConfigForMount_Default(t *testing.T) {
	dir := t.TempDir()
	localRoot := filepath.Join(dir, "defaultmount")
	if err := os.MkdirAll(localRoot, 0755); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		Version: "1",
		Mounts: []config.MountInstance{
			{
				Name:    "default-mount",
				Type:    "localfs",
				Default: true,
				Params: config.MountParams{
					"local_root": localRoot,
				},
				Encryption: &config.EncryptionConfig{
					Password: "testpass",
					Salt:     "testsalt",
				},
			},
		},
	}

	api, err := NewFileAPIFromConfigForMount(cfg, "", "", "")
	if err != nil {
		t.Fatalf("failed to create FileAPI for default mount: %v", err)
	}
	if api == nil {
		t.Fatal("expected FileAPI for default mount, got nil")
	}
}
