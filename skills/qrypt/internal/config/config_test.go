package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Quark.RootPath != "/Test" {
		t.Errorf("expected /Test, got %s", cfg.Quark.RootPath)
	}
	if cfg.Sync.MaxRetries != 3 {
		t.Errorf("expected 3, got %d", cfg.Sync.MaxRetries)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("expected debug, got %s", cfg.Log.Level)
	}
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"/abs/path", "/abs/path"},
		{"~", home},
		{"~/subdir", filepath.Join(home, "subdir")},
	}
	for _, tt := range tests {
		result := ExpandHome(tt.input)
		if result != tt.expected {
			t.Errorf("ExpandHome(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "qrypt.toml")
	content := `
[quark]
cookie = "test_cookie"
root_path = "/Test"

[encryption]
password = "pass"
salt = "salt"

[mount]
point = "~/QryptMount"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Quark.Cookie != "test_cookie" {
		t.Errorf("expected test_cookie, got %s", cfg.Quark.Cookie)
	}
	if cfg.Encryption.Password != "pass" {
		t.Errorf("expected pass, got %s", cfg.Encryption.Password)
	}
	if cfg.Encryption.Salt != "salt" {
		t.Errorf("expected salt, got %s", cfg.Encryption.Salt)
	}
}

func TestLoadConfig_NotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/qrypt.toml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadConfig_EmptyPath(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Quark.Cookie != "" {
		t.Errorf("expected empty cookie, got %s", cfg.Quark.Cookie)
	}
}

func TestFindConfigFile(t *testing.T) {
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)

	dir := t.TempDir()
	os.Chdir(dir)

	localPath := filepath.Join(dir, "qrypt.toml")
	os.WriteFile(localPath, []byte{}, 0o644)

	result := FindConfigFile()
	if result == "" {
		t.Error("expected non-empty config path")
	}
}

func TestFindConfigFile_NotFound(t *testing.T) {
	origWd, _ := os.Getwd()
	defer os.Chdir(origWd)

	dir := t.TempDir()
	os.Chdir(dir)

	result := FindConfigFile()
	if result != "" {
		t.Errorf("expected empty, got %s", result)
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
		wantErr  bool
	}{
		{"", 0, true},
		{"100", 100, false},
		{"1KB", 1024, false},
		{"2MB", 2 * 1024 * 1024, false},
		{"1GB", 1024 * 1024 * 1024, false},
		{"1TB", 1024 * 1024 * 1024 * 1024, false},
		{"10K", 10 * 1024, false},
		{"abc", 0, true},
	}
	for _, tt := range tests {
		result, err := ParseSize(tt.input)
		if tt.wantErr && err == nil {
			t.Errorf("ParseSize(%q) expected error", tt.input)
		}
		if !tt.wantErr && result != tt.expected {
			t.Errorf("ParseSize(%q) = %d, want %d", tt.input, result, tt.expected)
		}
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"", 0, true},
		{"5m", 5 * time.Minute, false},
		{"60s", 60 * time.Second, false},
		{"1h", time.Hour, false},
	}
	for _, tt := range tests {
		result, err := ParseDuration(tt.input)
		if tt.wantErr && err == nil {
			t.Errorf("ParseDuration(%q) expected error", tt.input)
		}
		if !tt.wantErr && result != tt.expected {
			t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}
