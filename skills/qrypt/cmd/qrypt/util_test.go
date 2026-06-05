package main

import (
	"testing"

	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

func TestResolveFullPath(t *testing.T) {
	tests := []struct {
		root     string
		userPath string
		want     string
	}{
		{"", "foo", "/foo"},
		{"/", "foo", "/foo"},
		{"/Test", "", "/Test"},
		{"/Test", "foo", "/Test/foo"},
		{"/Test", "/foo", "/Test/foo"},
		{"/Test", "a/b/c", "/Test/a/b/c"},
		{"/Test", "/a/b/c", "/Test/a/b/c"},
		{"", "", "/"},
	}
	for _, tt := range tests {
		got := config.ResolveFullPath(tt.root, tt.userPath)
		if got != tt.want {
			t.Errorf("ResolveFullPath(%q, %q) = %q, want %q", tt.root, tt.userPath, got, tt.want)
		}
	}
}

func TestMaskStr(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "(未设置)"},
		{"ab", "****"},
		{"abcd", "****"},
		{"abcde", "a****e"},
		{"abcdef", "a****f"},
		{"longpassword", "l****d"},
	}
	for _, tt := range tests {
		got := maskStr(tt.input)
		if got != tt.want {
			t.Errorf("maskStr(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveMountConfig(t *testing.T) {
	t.Run("valid mount name", func(t *testing.T) {
		cfg := &config.Config{
			Mounts: []config.MountInstance{
				{
					Name:    "mount-a",
					Type:    "quark",
					Default: false,
					Params:  config.MountParams{"cookie": "cookie-a"},
				},
				{
					Name:    "mount-b",
					Type:    "localfs",
					Default: true,
					Params:  config.MountParams{"local_root": "/tmp/b"},
				},
			},
		}
		rc, err := resolveMountConfig(cfg, "mount-a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rc == nil {
			t.Fatal("expected ResolvedMountConfig, got nil")
		}
		if rc.Name != "mount-a" {
			t.Errorf("expected name mount-a, got %s", rc.Name)
		}
		if rc.Type != "quark" {
			t.Errorf("expected type quark, got %s", rc.Type)
		}
	})

	t.Run("empty mount name falls back to default", func(t *testing.T) {
		cfg := &config.Config{
			Mounts: []config.MountInstance{
				{
					Name:    "default-mount",
					Type:    "localfs",
					Default: true,
					Params:  config.MountParams{"local_root": "/tmp/default"},
				},
			},
		}
		rc, err := resolveMountConfig(cfg, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rc == nil {
			t.Fatal("expected ResolvedMountConfig, got nil")
		}
		if rc.Name != "default-mount" {
			t.Errorf("expected name default-mount, got %s", rc.Name)
		}
	})

	t.Run("non-existent mount name returns error", func(t *testing.T) {
		cfg := &config.Config{
			Mounts: []config.MountInstance{
				{
					Name:    "existing",
					Type:    "localfs",
					Default: true,
					Params:  config.MountParams{"local_root": "/tmp/existing"},
				},
			},
		}
		_, err := resolveMountConfig(cfg, "missing")
		if err == nil {
			t.Fatal("expected error for missing mount, got nil")
		}
	})
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{2048, "2.0 KB"},
		{1024 * 1024, "1.0 MB"},
		{5 * 1024 * 1024, "5.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{10 * 1024 * 1024 * 1024, "10.0 GB"},
		{1500, "1.5 KB"},
		{1500000, "1.4 MB"},
	}
	for _, tt := range tests {
		got := formatBytes(tt.n)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}


