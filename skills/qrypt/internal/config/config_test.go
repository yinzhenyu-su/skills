package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Version != "1" {
		t.Errorf("expected version 1, got %s", cfg.Version)
	}
	if cfg.Defaults.Sync.ConcurrentUploads != 3 {
		t.Errorf("expected 3, got %d", cfg.Defaults.Sync.ConcurrentUploads)
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
version = "1"

[[mounts]]
name = "test"
type = "quark"
mount_point = "~/QryptMount"

[mounts.params]
cookie = "test_cookie"
root_path = "/"

[mounts.encryption]
password = "pass"
salt = "salt"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, vr, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(cfg.Mounts))
	}
	if cfg.Mounts[0].Params.Cookie != "test_cookie" {
		t.Errorf("expected test_cookie, got %s", cfg.Mounts[0].Params.Cookie)
	}
	if vr == nil {
		t.Fatal("expected validation result")
	}
}

func TestLoadConfig_NotFound(t *testing.T) {
	_, _, err := LoadConfig("/nonexistent/qrypt.toml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadConfig_EmptyPath(t *testing.T) {
	cfg, vr, err := LoadConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != "1" {
		t.Errorf("expected version 1, got %s", cfg.Version)
	}
	if vr == nil {
		t.Fatal("expected validation result")
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

func TestDefaultConfig_ValidTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")
	if err := WriteDefaultConfig(path); err != nil {
		t.Fatalf("WriteDefaultConfig failed: %v", err)
	}
	cfg, vr, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if vr.Valid {
		t.Log("default config is valid")
	} else {
		for _, c := range vr.Checks {
			t.Logf("  %s: %s — %s", c.Field, c.Status, c.Message)
		}
	}
	if len(cfg.Mounts) == 0 {
		t.Error("default config should have at least one mount")
	}
}

func TestValidateConfig_NoMounts(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mounts = nil
	vr := ValidateConfig(cfg)
	if vr.Valid {
		t.Error("expected validation to fail with no mounts")
	}
}

func TestValidateConfig_BadDefaults(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Defaults.Cache.MaxSize = "invalid"
	cfg.Defaults.Sync.ConcurrentUploads = 0
	cfg.Mounts = []MountInstance{
		{
			Name:       "test",
			Type:       "quark",
			MountPoint: "~/Qrypt/Test",
			Params:     MountParams{Cookie: "c"},
			Encryption: &EncryptionConfig{Password: "p"},
		},
	}
	vr := ValidateConfig(cfg)
	if vr.Valid {
		t.Error("expected validation to fail due to bad defaults")
	}
}

func TestValidMountName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"personal", true},
		{"my-mount", true},
		{"a", true},
		{"a-1", true},
		{"Personal", false},
		{"my_mount", false},
		{"", false},
		{"this-name-is-way-too-long-for-validation", false},
	}
	for _, tt := range tests {
		got := ValidMountName(tt.name)
		if got != tt.valid {
			t.Errorf("ValidMountName(%q) = %v, want %v", tt.name, got, tt.valid)
		}
	}
}

func TestRootPathForMount_AllTypes(t *testing.T) {
	tests := []struct {
		m    MountInstance
		want string
	}{
		{MountInstance{Type: "quark", Params: MountParams{RootPath: "/MyPath"}}, "/MyPath"},
		{MountInstance{Type: "quark"}, "/"},
		{MountInstance{Type: "yun139", Params: MountParams{RootID: "123"}}, "123"},
		{MountInstance{Type: "localfs", Params: MountParams{LocalRoot: "/data"}}, "/data"},
	}
	for _, tt := range tests {
		got := RootPathForMount(tt.m)
		if got != tt.want {
			t.Errorf("RootPathForMount(%+v) = %q, want %q", tt.m, got, tt.want)
		}
	}
}

func TestLoadConfig_NewFormatTwoMounts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "qrypt.toml")
	content := `
version = "1"

[[mounts]]
name = "personal"
type = "quark"
mount_point = "~/Qrypt/A"

[mounts.params]
cookie = "cookie_a"

[[mounts]]
name = "work"
type = "quark"
mount_point = "~/Qrypt/B"
enabled = false

[mounts.params]
cookie = "cookie_b"

[mounts.encryption]
password = "work_pass"

[defaults.encryption]
password = "default_pass"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, _, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Mounts) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(cfg.Mounts))
	}
	if cfg.Mounts[0].Name != "personal" || cfg.Mounts[0].Params.Cookie != "cookie_a" {
		t.Errorf("bad first mount: %+v", cfg.Mounts[0])
	}
	if cfg.Mounts[1].Name != "work" || cfg.Mounts[1].Params.Cookie != "cookie_b" {
		t.Errorf("bad second mount: %+v", cfg.Mounts[1])
	}
}

func TestMergeInstanceConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Defaults.Encryption.Password = "global_pass"
	cfg.Defaults.Sync.ConcurrentUploads = 5

	m := MountInstance{
		Name:       "test",
		Type:       "quark",
		MountPoint: "~/Qrypt/Test",
		Params: MountParams{Cookie: "test_cookie", RootPath: "/"},
	}
	rc := cfg.MergeInstanceConfig(m)
	if rc.Encryption.Password != "global_pass" {
		t.Errorf("expected global_pass, got %s", rc.Encryption.Password)
	}
	if rc.Sync.ConcurrentUploads != 5 {
		t.Errorf("expected 5, got %d", rc.Sync.ConcurrentUploads)
	}

	// Mount-level override
	pass := "local_pass"
	m.Encryption = &EncryptionConfig{Password: pass}
	rc = cfg.MergeInstanceConfig(m)
	if rc.Encryption.Password != "local_pass" {
		t.Errorf("expected local_pass, got %s", rc.Encryption.Password)
	}
}

func TestMergeInstanceConfig_EncryptionDefaults(t *testing.T) {
	cfg := DefaultConfig()

	// 无 mount 级 config → 使用全局默认值
	m := MountInstance{
		Name:       "test",
		Type:       "quark",
		MountPoint: "~/Qrypt/Test",
		Params:     MountParams{Cookie: "test_cookie", RootPath: "/"},
	}
	rc := cfg.MergeInstanceConfig(m)
	if rc.Encryption.FileNameEncryption != "standard" {
		t.Errorf("expected default standard, got %s", rc.Encryption.FileNameEncryption)
	}
	if rc.Encryption.FileNameEncoding != "base32" {
		t.Errorf("expected default base32, got %s", rc.Encryption.FileNameEncoding)
	}

	// mount 级覆盖
	m.Encryption = &EncryptionConfig{
		Password:           "p",
		FileNameEncryption: "obfuscate",
		FileNameEncoding:   "base64",
	}
	rc = cfg.MergeInstanceConfig(m)
	if rc.Encryption.FileNameEncryption != "obfuscate" {
		t.Errorf("expected obfuscate, got %s", rc.Encryption.FileNameEncryption)
	}
	if rc.Encryption.FileNameEncoding != "base64" {
		t.Errorf("expected base64, got %s", rc.Encryption.FileNameEncoding)
	}

	// 空字段 → 默认值
	m.Encryption = &EncryptionConfig{
		Password: "p",
	}
	rc = cfg.MergeInstanceConfig(m)
	if rc.Encryption.FileNameEncryption != "standard" {
		t.Errorf("empty should default to standard, got %s", rc.Encryption.FileNameEncryption)
	}
	if rc.Encryption.FileNameEncoding != "base32" {
		t.Errorf("empty should default to base32, got %s", rc.Encryption.FileNameEncoding)
	}
}

func TestLoadConfig_OldFormatRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "qrypt.toml")
	content := `
[quark]
cookie = "old_cookie"

[encryption]
password = "old_pass"

[mount]
point = "~/Qrypt"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, vr, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Mounts) != 0 {
		t.Errorf("expected 0 mounts for old-format config, got %d", len(cfg.Mounts))
	}
	if vr.Valid {
		t.Error("expected validation to fail: old format has no [[mounts]]")
	}
}

func TestValidateConfig_DuplicateMountName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "qrypt.toml")
	content := `
[[mounts]]
name = "dup"
type = "quark"
mount_point = "~/Qrypt/A"

[mounts.params]
cookie = "a"

[[mounts]]
name = "dup"
type = "quark"
mount_point = "~/Qrypt/B"

[mounts.params]
cookie = "b"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, vr, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if vr.Valid {
		t.Error("expected validation to fail due to duplicate mount name")
	}
}

func TestValidateConfig_InvalidMountName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "qrypt.toml")
	content := `
[[mounts]]
name = "Invalid_Name!"
type = "quark"
mount_point = "~/Qrypt/A"

[mounts.params]
cookie = "a"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, vr, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if vr.Valid {
		t.Error("expected validation to fail due to invalid mount name")
	}
}

func TestFindDefaultMount_Explicit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mounts = []MountInstance{
		{Name: "a", Type: "quark", Params: MountParams{Cookie: "a"}, MountPoint: "~/A"},
		{Name: "b", Type: "quark", Default: true, Params: MountParams{Cookie: "b"}, MountPoint: "~/B"},
		{Name: "c", Type: "quark", Params: MountParams{Cookie: "c"}, MountPoint: "~/C"},
	}
	m := FindDefaultMount(cfg)
	if m == nil || m.Name != "b" {
		t.Errorf("expected 'b' (explicit default), got %v", m)
	}
}

func TestFindDefaultMount_FirstEnabled(t *testing.T) {
	cfg := DefaultConfig()
	f := false
	t2 := true
	cfg.Mounts = []MountInstance{
		{Name: "a", Type: "quark", Enabled: &f, Params: MountParams{Cookie: "a"}, MountPoint: "~/A"},
		{Name: "b", Type: "quark", Enabled: &t2, Params: MountParams{Cookie: "b"}, MountPoint: "~/B"},
		{Name: "c", Type: "quark", Params: MountParams{Cookie: "c"}, MountPoint: "~/C"},
	}
	m := FindDefaultMount(cfg)
	if m == nil || m.Name != "b" {
		t.Errorf("expected 'b' (first enabled), got %v", m)
	}
}

func TestFindDefaultMount_AllDisabled_FirstMount(t *testing.T) {
	cfg := DefaultConfig()
	f := false
	cfg.Mounts = []MountInstance{
		{Name: "a", Type: "quark", Enabled: &f, Params: MountParams{Cookie: "a"}, MountPoint: "~/A"},
		{Name: "b", Type: "quark", Enabled: &f, Params: MountParams{Cookie: "b"}, MountPoint: "~/B"},
	}
	m := FindDefaultMount(cfg)
	if m != nil {
		t.Errorf("expected nil (all disabled), got %v", m)
	}
}

func TestFindDefaultMount_Empty(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mounts = nil
	m := FindDefaultMount(cfg)
	if m != nil {
		t.Errorf("expected nil, got %v", m)
	}
}

func TestValidateConfig_DuplicateDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "qrypt.toml")
	content := `
[[mounts]]
name = "a"
type = "quark"
mount_point = "~/A"
default = true

[mounts.params]
cookie = "a"

[[mounts]]
name = "b"
type = "quark"
mount_point = "~/B"
default = true

[mounts.params]
cookie = "b"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	_, vr, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if vr.Valid {
		t.Fatal("expected validation to fail due to duplicate default = true")
	}
	hasDefaultErr := false
	for _, c := range vr.Checks {
		if c.Field == "mounts" && c.Status == "error" {
			hasDefaultErr = true
			break
		}
	}
	if !hasDefaultErr {
		t.Fatal("expected error check for duplicate defaults")
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
