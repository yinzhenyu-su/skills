package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

func TestCountLines(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		content string
		want    int
	}{
		{"", 0},
		{"a\n", 2},    // 1 newline → 2 lines
		{"a\nb\n", 3}, // 2 newlines → 3 lines
		{"a\nb\nc", 3},
		{"line1\nline2\nline3\n", 4}, // 3 newlines → 4 lines
	}
	for _, tt := range tests {
		path := filepath.Join(dir, "test.txt")
		if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
			t.Fatal(err)
		}
		got := countLines(path)
		if got != tt.want {
			t.Errorf("countLines(%q) = %d, want %d", tt.content, got, tt.want)
		}
	}
}

func TestCountLinesFileNotFound(t *testing.T) {
	got := countLines("/nonexistent/path/file.txt")
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

func TestFormatBytesEdgeCases(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{-1, "-1 B"},
		{1 << 40, "1.0 TB"},
	}
	for _, tt := range tests {
		got := formatBytes(tt.n)
		if got != tt.want {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestParseMountPath(t *testing.T) {
	tests := []struct {
		input     string
		wantMount string
		wantPath  string
	}{
		{"personal:/docs/file.txt", "personal", "/docs/file.txt"},
		{"my-mount:/path", "my-mount", "/path"},
		{"a:/", "a", "/"},
		{"/abs/path", "", "/abs/path"},
		{"./relative", "", "./relative"},
		{"~/home/path", "", "~/home/path"},
		{"just-a-path", "", "just-a-path"},
		{"C:/Windows", "", "C:/Windows"},
	}
	for _, tt := range tests {
		mount, path := ParseMountPath(tt.input)
		if mount != tt.wantMount {
			t.Errorf("ParseMountPath(%q) mount = %q, want %q", tt.input, mount, tt.wantMount)
		}
		if path != tt.wantPath {
			t.Errorf("ParseMountPath(%q) path = %q, want %q", tt.input, path, tt.wantPath)
		}
	}
}

func TestResolveMount(t *testing.T) {
	origExit := osExit
	defer func() { osExit = origExit }()

	cmd := &cobra.Command{}
	cmd.Flags().String("mount", "", "")

	tests := []struct {
		name      string
		mountFlag string
		path      string
		wantMount string
		wantPath  string
		wantExit  bool
	}{
		{
			name:      "flag and prefix agree - strip prefix",
			mountFlag: "quark",
			path:      "quark:/docs/file.txt",
			wantMount: "quark",
			wantPath:  "/docs/file.txt",
		},
		{
			name:      "flag and prefix conflict - error exit",
			mountFlag: "quark",
			path:      "other:/docs/file.txt",
			wantMount: "",
			wantPath:  "",
			wantExit:  true,
		},
		{
			name:      "flag set, no prefix - use flag",
			mountFlag: "quark",
			path:      "/docs/file.txt",
			wantMount: "quark",
			wantPath:  "/docs/file.txt",
		},
		{
			name:      "no flag, prefix present - use prefix and strip",
			mountFlag: "",
			path:      "personal:/docs/file.txt",
			wantMount: "personal",
			wantPath:  "/docs/file.txt",
		},
		{
			name:      "no flag, no prefix - empty mount",
			mountFlag: "",
			path:      "/docs/file.txt",
			wantMount: "",
			wantPath:  "/docs/file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd.Flags().Set("mount", tt.mountFlag)

			pathCopy := tt.path
			pathPtr := &pathCopy

			var mount string
			var didExit bool

			if tt.wantExit {
				osExit = func(code int) {
					didExit = true
					panic(code)
				}
				func() {
					defer func() {
						if r := recover(); r != nil {
							if _, ok := r.(int); !ok {
								panic(r)
							}
						}
					}()
					mount = resolveMount(cmd, pathPtr)
				}()
				if !didExit {
					t.Errorf("expected exit but did not exit")
				}
				return
			}

			mount = resolveMount(cmd, pathPtr)

			if mount != tt.wantMount {
				t.Errorf("mount = %q, want %q", mount, tt.wantMount)
			}
			if pathCopy != tt.wantPath {
				t.Errorf("path = %q, want %q", pathCopy, tt.wantPath)
			}
		})
	}
}

func TestStripRootPath(t *testing.T) {
	tests := []struct {
		rootPath    string
		displayPath string
		want        string
	}{
		{"/Test", "/Test/docs/file.txt", "/docs/file.txt"},
		{"/Test", "/docs/file.txt", "/docs/file.txt"},
		{"/", "/docs/file.txt", "/docs/file.txt"},
		{"", "/docs/file.txt", "/docs/file.txt"},
		{"/MyDrive/Encrypt", "/MyDrive/Encrypt/docs", "/docs"},
		{"/Test", "/Test", "/"},
		{"/Test", "/TestFile", "/TestFile"},
	}
	for _, tt := range tests {
		got := StripRootPath(tt.rootPath, tt.displayPath)
		if got != tt.want {
			t.Errorf("StripRootPath(%q, %q) = %q, want %q", tt.rootPath, tt.displayPath, got, tt.want)
		}
	}
}

func TestFormatRemotePath(t *testing.T) {
	tests := []struct {
		name       string
		mountName  string
		remotePath string
		want       string
	}{
		{"with mount", "quark", "/docs/file.txt", "quark:/docs/file.txt"},
		{"empty mount", "", "/docs/file.txt", "/docs/file.txt"},
		{"root path", "quark", "/", "quark:/"},
		{"no leading slash", "quark", "docs/file.txt", "quark:/docs/file.txt"},
		{"empty path", "quark", "", "quark:/"},
	}
	for _, tt := range tests {
		got := formatRemotePath(tt.mountName, tt.remotePath)
		if got != tt.want {
			t.Errorf("formatRemotePath(%q, %q) = %q, want %q", tt.mountName, tt.remotePath, got, tt.want)
		}
	}
}

func TestStripRootPathAndFormatRemotePath(t *testing.T) {
	tests := []struct {
		name       string
		rootPath   string
		remotePath string
		mountName  string
		want       string
	}{
		{
			name:       "strip root then format with mount",
			rootPath:   "/Test",
			remotePath: "/Test/docs/file.txt",
			mountName:  "quark",
			want:       "quark:/docs/file.txt",
		},
		{
			name:       "strip root then format empty mount",
			rootPath:   "/Test",
			remotePath: "/Test/docs/file.txt",
			mountName:  "",
			want:       "/docs/file.txt",
		},
		{
			name:       "no stripping needed - path outside root",
			rootPath:   "/Test",
			remotePath: "/docs/file.txt",
			mountName:  "quark",
			want:       "quark:/docs/file.txt",
		},
		{
			name:       "root path exact match",
			rootPath:   "/Test",
			remotePath: "/Test",
			mountName:  "quark",
			want:       "quark:/",
		},
		{
			name:       "deeply nested with custom root",
			rootPath:   "/MyDrive/Encrypt",
			remotePath: "/MyDrive/Encrypt/a/b/c/file.txt",
			mountName:  "personal",
			want:       "personal:/a/b/c/file.txt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stripped := StripRootPath(tt.rootPath, tt.remotePath)
			got := formatRemotePath(tt.mountName, stripped)
			if got != tt.want {
				t.Errorf("StripRootPath(%q, %q) = %q, formatRemotePath(%q, %q) = %q, want %q",
					tt.rootPath, tt.remotePath, stripped, tt.mountName, stripped, got, tt.want)
			}
		})
	}
}

func TestStripRootPathAndFormatRemotePathPipeline(t *testing.T) {
	tests := []struct {
		name        string
		mountName   string
		rootPath    string
		displayPath string
		want        string
	}{
		{"quark mount strips root", "quark", "/Test", "/Test/docs/file.txt", "quark:/docs/file.txt"},
		{"quark mount root exact", "quark", "/Test", "/Test", "quark:/"},
		{"empty mount strips root", "", "/Test", "/Test/subdir", "/subdir"},
		{"personal mount strips root", "personal", "/MyDrive", "/MyDrive/files/doc.pdf", "personal:/files/doc.pdf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stripped := StripRootPath(tt.rootPath, tt.displayPath)
			got := formatRemotePath(tt.mountName, stripped)
			if got != tt.want {
				t.Errorf("StripRootPath(%q, %q) = %q, formatRemotePath(%q, %q) = %q, want %q",
					tt.rootPath, tt.displayPath, stripped, tt.mountName, stripped, got, tt.want)
			}
		})
	}
}

func TestResolveMountConfigWithFixture(t *testing.T) {
	cfg := &config.Config{
		Version: "1",
		Mounts: []config.MountInstance{
			{
				Name:       "primary",
				Type:       "localfs",
				MountPoint: "~/QryptPrimary",
				Default:    true,
				Params: config.MountParams{
					LocalRoot: "/data/primary",
				},
			},
			{
				Name:       "secondary",
				Type:       "localfs",
				MountPoint: "~/QryptSecondary",
				Params: config.MountParams{
					LocalRoot: "/data/secondary",
				},
			},
		},
	}

	t.Run("find by name", func(t *testing.T) {
		rc, err := resolveMountConfig(cfg, "secondary")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rc.Name != "secondary" {
			t.Errorf("name = %q, want %q", rc.Name, "secondary")
		}
		if rc.Type != "localfs" {
			t.Errorf("type = %q, want %q", rc.Type, "localfs")
		}
		if rc.Params.LocalRoot != "/data/secondary" {
			t.Errorf("local_root = %q, want %q", rc.Params.LocalRoot, "/data/secondary")
		}
	})

	t.Run("empty falls back to default", func(t *testing.T) {
		rc, err := resolveMountConfig(cfg, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rc.Name != "primary" {
			t.Errorf("name = %q, want %q", rc.Name, "primary")
		}
		if rc.Params.LocalRoot != "/data/primary" {
			t.Errorf("local_root = %q, want %q", rc.Params.LocalRoot, "/data/primary")
		}
	})

	t.Run("nonexistent returns error", func(t *testing.T) {
		_, err := resolveMountConfig(cfg, "nonexistent")
		if err == nil {
			t.Fatal("expected error for nonexistent mount")
		}
	})

	t.Run("combined with StripRootPath", func(t *testing.T) {
		rc, err := resolveMountConfig(cfg, "primary")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		mountInstance := config.MountInstance{
			Type:   rc.Type,
			Params: rc.Params,
		}
		rootPath := config.RootPathForMount(mountInstance)
		if rootPath != "/data/primary" {
			t.Errorf("rootPath = %q, want %q", rootPath, "/data/primary")
		}

		displayPath := "/data/primary/docs/file.txt"
		stripped := StripRootPath(rootPath, displayPath)
		if stripped != "/docs/file.txt" {
			t.Errorf("StripRootPath(%q, %q) = %q, want %q", rootPath, displayPath, stripped, "/docs/file.txt")
		}
	})
}

func TestFullPathResolutionFlow(t *testing.T) {
	localRoot := t.TempDir()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "qrypt.toml")
	content := fmt.Sprintf(`version = "1"

[[mounts]]
name = "testmount"
type = "localfs"
mount_point = "/tmp/testmount"
default = true

[mounts.params]
local_root = %q

[defaults.encryption]
password = "testpass"
salt = "testsalt"
`, localRoot)
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "")
	cmd.Flags().String("mount", "", "")
	cmd.Flags().String("password", "", "")
	cmd.Flags().String("salt", "", "")
	if err := cmd.Flags().Set("config", cfgPath); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		mountFlag  string
		inputPath  string
		wantMount  string
		wantPath   string
		wantFormat string
	}{
		{
			name:       "prefix with mount flag",
			mountFlag:  "testmount",
			inputPath:  "testmount:/docs/file.txt",
			wantMount:  "testmount",
			wantPath:   "/docs/file.txt",
			wantFormat: "testmount:/docs/file.txt",
		},
		{
			name:       "prefix only",
			mountFlag:  "",
			inputPath:  "testmount:/docs/file.txt",
			wantMount:  "testmount",
			wantPath:   "/docs/file.txt",
			wantFormat: "testmount:/docs/file.txt",
		},
		{
			name:       "flag only no prefix",
			mountFlag:  "testmount",
			inputPath:  "/docs/file.txt",
			wantMount:  "testmount",
			wantPath:   "/docs/file.txt",
			wantFormat: "testmount:/docs/file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := cmd.Flags().Set("mount", tt.mountFlag); err != nil {
				t.Fatal(err)
			}

			pathCopy := tt.inputPath
			pathPtr := &pathCopy

			mountName := resolveMount(cmd, pathPtr)
			if mountName != tt.wantMount {
				t.Errorf("resolveMount mount = %q, want %q", mountName, tt.wantMount)
			}
			if pathCopy != tt.wantPath {
				t.Errorf("resolveMount path = %q, want %q", pathCopy, tt.wantPath)
			}

			api, err := apiFromCmdForMount(cmd, mountName)
			if err != nil {
				t.Fatalf("apiFromCmdForMount error: %v", err)
			}
			if api == nil {
				t.Fatal("apiFromCmdForMount returned nil api")
			}

			cfg, err := getCfg(cfgPath)
			if err != nil {
				t.Fatalf("getCfg error: %v", err)
			}
			mountCfg, err := resolveMountConfig(cfg, mountName)
			if err != nil {
				t.Fatalf("resolveMountConfig error: %v", err)
			}
			mountInstance := config.MountInstance{
				Type:   mountCfg.Type,
				Params: mountCfg.Params,
			}
			rootPath := config.RootPathForMount(mountInstance)

			displayPath := rootPath + "/docs/file.txt"
			stripped := StripRootPath(rootPath, displayPath)
			got := formatRemotePath(mountName, stripped)
			if got != tt.wantFormat {
				t.Errorf("formatRemotePath(%q, %q) = %q, want %q", mountName, stripped, got, tt.wantFormat)
			}
		})
	}
}