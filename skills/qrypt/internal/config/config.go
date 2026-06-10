package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/yinzhenyu/skills/qrypt/cipher"
	"github.com/yinzhenyu/skills/qrypt/drivers"
)

//go:embed default.toml
var defaultConfigContent string

var validMountName = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

func ValidMountName(s string) bool {
	return validMountName.MatchString(s)
}

const CurrentVersion = "1"

type Config struct {
	// Schema version.
	Version string `toml:"version"`

	// Working directory for cache, logs, and socket.
	// Defaults to ~/.qrypt or $QRYPT_WORK_DIR.
	// This does NOT affect config file discovery — see FindConfigFile.
	WorkDir string `toml:"work_dir"`

	// Mount instances.
	Mounts   []MountInstance `toml:"mounts"`
	Defaults DefaultsConfig  `toml:"defaults"`

	// Shared config (not per-mount).
	Log LogConfig `toml:"log"`

	// Legacy fields (no TOML tags — no longer read from config files).
	Mount MountConfig

	Encryption EncryptionConfig
	Cache      CacheConfig
	Sync       SyncConfig

	// computed: resolved after loading config
	effectiveWorkDir string
}

// DefaultsConfig holds global default values inherited by each mount instance.
type DefaultsConfig struct {
	Encryption EncryptionConfig `toml:"encryption"`
	Sync       SyncConfig       `toml:"sync"`
	Cache      CacheConfig      `toml:"cache"`
	Mount      MountConfig      `toml:"mount"`
}

// MountInstance declares one cloud drive mount.
type MountInstance struct {
	Name       string      `toml:"name"`        // unique identifier, [a-z0-9-]{1,32}
	Type       string      `toml:"type"`        // "quark" | "yun139" | "localfs"
	MountPoint string      `toml:"mount_point"` // FUSE mount path
	Enabled    *bool       `toml:"enabled"`     // nil = true
	Default    bool        `toml:"default"`     // true = selection target when --mount omitted
	AllowOther bool        `toml:"allow_other"`
	VolName    string      `toml:"volname"`     // macOS Finder volume name (default: QryptDrive)

	Params     MountParams        `toml:"params"`
	Encryption *EncryptionConfig  `toml:"encryption"` // nil = use defaults
	Sync       *SyncConfig        `toml:"sync"`       // nil = use defaults
	Cache      *CacheConfig       `toml:"cache"`      // nil = use defaults

	// TestEnabled marks this mount for inclusion in integration tests.
	// When running `go test -tags=integration`, mounts with test_enabled = true
	// are discovered and tested against the real cloud drive.
	TestEnabled *bool `toml:"test_enabled"` // nil = false
}

// IsTestEnabled reports whether this mount is included in integration tests.
func (m MountInstance) IsTestEnabled() bool {
	return m.TestEnabled != nil && *m.TestEnabled
}

// MountParams holds driver-specific configuration as a flat key-value map.
// Keys correspond to the driver's registered ParamSpec.Key values.
// TOML decodes [mounts.params] sections directly into this map.
type MountParams = map[string]string

// ResolvedMountConfig is a mount instance with all defaults merged in.
type ResolvedMountConfig struct {
	Name       string
	Type       string
	MountPoint string
	AllowOther bool
	VolName    string
	Enabled    bool
	Params     MountParams

	Encryption EncryptionConfig
	Sync       SyncConfig
	Cache      CacheConfig
	CacheDir   string // computed: ~/.qrypt/cache/<name>/
}



type EncryptionConfig struct {
	Password           string `toml:"password"`
	Salt               string `toml:"salt"`
	FileNameEncryption string `toml:"filename_encryption"`
	FileNameEncoding   string `toml:"filename_encoding"`
}

type CacheConfig struct {
	Dir            string `toml:"-"`      // computed: $QRYPT_WORK_DIR/cache
	MaxSize        string `toml:"max_size"`
	MemCacheSizeMB int    `toml:"mem_cache_size_mb"`
}

type MountConfig struct {
	Point      string
	AllowOther bool
	VolName    string `toml:"volname"`
}

type SyncConfig struct {
	MaxRetries        int    `toml:"max_retries"`
	ConcurrentUploads int    `toml:"concurrent_uploads"`
	DirCacheTTL       string `toml:"dir_cache_ttl"`
	WriteBackTimeout  string `toml:"write_back_timeout"` // FUSE 写入后延迟上传时间 (e.g. "5s", "0s"=立即)
}

type LogConfig struct {
	Level      string `toml:"level"`
	File       string `toml:"-"`      // computed: $QRYPT_WORK_DIR/qrypt.log
	MaxSize    int    `toml:"max_size"`    // MB, single log file max before rotation
	MaxBackups int    `toml:"max_backups"` // count, old log files to retain
	MaxAge     int    `toml:"max_age"`     // days, old log files to retain
	Compress   *bool  `toml:"compress"`
}

func DefaultConfig() *Config {
	workDir := WorkDir()
	cfg := &Config{
		Version: CurrentVersion,
		Log: LogConfig{
			Level: "debug",
			File:  filepath.Join(workDir, "qrypt.log"),
		},
		Defaults: DefaultsConfig{
			Encryption: EncryptionConfig{},
			Sync: SyncConfig{
				MaxRetries:        3,
				ConcurrentUploads: 3,
				DirCacheTTL:       "5m",
				WriteBackTimeout:  "0s",
			},
			Cache: CacheConfig{
				MaxSize: "10GB",
			},
		},
	}
	cfg.resolveWorkDir()
	return cfg
}

// resolveWorkDir sets effectiveWorkDir based on config or default.
func (c *Config) resolveWorkDir() {
	if c.WorkDir != "" {
		c.effectiveWorkDir = ExpandHome(c.WorkDir)
	} else {
		c.effectiveWorkDir = WorkDir()
	}
}

// EffectiveWorkDir returns the resolved working directory.
// Priority: config file's work_dir > $QRYPT_WORK_DIR > ~/.qrypt
func (c *Config) EffectiveWorkDir() string {
	return c.effectiveWorkDir
}

// DiscoverTestMounts returns mounts with test_enabled = true,
// suitable for integration test suite.
func (c *Config) DiscoverTestMounts() []MountInstance {
	var result []MountInstance
	for _, m := range c.Mounts {
		if m.IsTestEnabled() {
			result = append(result, m)
		}
	}
	return result
}

// MergeInstanceConfig applies global defaults to a MountInstance and returns a resolved config.
func (c *Config) MergeInstanceConfig(m MountInstance) *ResolvedMountConfig {
	enabled := true
	if m.Enabled != nil {
		enabled = *m.Enabled
	}

	enc := c.Defaults.Encryption
	if m.Encryption != nil {
		enc = *m.Encryption
	}
	if enc.FileNameEncryption == "" {
		enc.FileNameEncryption = "standard"
	}
	if enc.FileNameEncoding == "" {
		enc.FileNameEncoding = "base32"
	}

	sync := c.Defaults.Sync
	if m.Sync != nil {
		sync = *m.Sync
	}

	cache := c.Defaults.Cache
	if m.Cache != nil {
		cache = *m.Cache
	}

	cacheDir := filepath.Join(c.EffectiveWorkDir(), "cache", m.Name)

	volName := m.VolName
	if volName == "" {
		volName = c.Defaults.Mount.VolName
	}
	if volName == "" {
		volName = "QryptDrive"
	}

	return &ResolvedMountConfig{
		Name:       m.Name,
		Type:       m.Type,
		MountPoint: ExpandHome(m.MountPoint),
		AllowOther: m.AllowOther,
		VolName:    volName,
		Enabled:    enabled,
		Params:     m.Params,
		Encryption: enc,
		Sync:       sync,
		Cache:      cache,
		CacheDir:   cacheDir,
	}
}

// WorkDir returns the qrypt working directory.
// It is set by QRYPT_WORK_DIR environment variable, defaulting to ~/.qrypt.
func WorkDir() string {
	if dir := os.Getenv("QRYPT_WORK_DIR"); dir != "" {
		return ExpandHome(dir)
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".qrypt")
}

// WriteDefaultConfig generates a default config file at path.
func WriteDefaultConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(defaultConfigContent); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func ExpandHome(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if len(path) == 1 {
		return homeDir
	}
	return filepath.Join(homeDir, path[2:])
}

func LoadConfig(path string) (*Config, *ValidationResult, error) {
	config := DefaultConfig()
	if path == "" {
		result := ValidateConfig(config)
		return config, result, nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("config file not found: %s", path)
	}
	if _, err := toml.DecodeFile(path, config); err != nil {
		return nil, nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	config.resolveWorkDir()
	config.Log.File = filepath.Join(config.effectiveWorkDir, "qrypt.log")

	result := ValidateConfig(config)
	return config, result, nil
}

func FindConfigFile() string {
	candidates := []string{"qrypt.toml"}
	if homeDir, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(homeDir, ".config", "qrypt", "qrypt.toml"),
			filepath.Join(homeDir, ".qrypt.toml"),
		)
	}
	// Add $QRYPT_WORK_DIR/qrypt.toml
	if workDir := os.Getenv("QRYPT_WORK_DIR"); workDir != "" {
		candidates = append(candidates, filepath.Join(ExpandHome(workDir), "qrypt.toml"))
	}
	candidates = append(candidates, "/etc/qrypt/qrypt.toml")
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// RootPath returns the root path/ID for the default mount instance.
func (c *Config) RootPath() string {
	if m := FindDefaultMount(c); m != nil {
		return RootPathForMount(*m)
	}
	return "/"
}

// FindDefaultMount returns the default mount:
//   - the mount with default = true (if exactly one)
//   - otherwise the first enabled mount
//   - nil if no mounts are enabled (or no mounts exist)
func FindDefaultMount(cfg *Config) *MountInstance {
	for _, m := range cfg.Mounts {
		if m.Default {
			return &m
		}
	}
	for _, m := range cfg.Mounts {
		enabled := true
		if m.Enabled != nil {
			enabled = *m.Enabled
		}
		if enabled {
			return &m
		}
	}
	return nil
}

// RootPathForMount returns the root path/ID for a specific mount instance.
func RootPathForMount(m MountInstance) string {
	if meta, ok := drivers.GetMeta(m.Type); ok && meta.RootKey != "" {
		if v := m.Params[meta.RootKey]; v != "" {
			return v
		}
	}
	return "/"
}

func ParseSize(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}
	var multiplier int64 = 1
	var numStr string
	suffixes := map[string]int64{
		"KB": 1024, "MB": 1024 * 1024, "GB": 1024 * 1024 * 1024, "TB": 1024 * 1024 * 1024 * 1024,
		"K": 1024, "M": 1024 * 1024, "G": 1024 * 1024 * 1024, "T": 1024 * 1024 * 1024 * 1024,
	}
	for suffix, mult := range suffixes {
		if len(s) > len(suffix) && s[len(s)-len(suffix):] == suffix {
			multiplier = mult
			numStr = s[:len(s)-len(suffix)]
			break
		}
	}
	if numStr == "" {
		numStr = s
	}
	var num int64
	if _, err := fmt.Sscanf(numStr, "%d", &num); err != nil {
		return 0, fmt.Errorf("invalid size format: %s", s)
	}
	return num * multiplier, nil
}

func ParseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty duration string")
	}
	return time.ParseDuration(s)
}

// ResolveFullPath resolves a user-provided path against a root path.
func ResolveFullPath(rootPath, userPath string) string {
	root := strings.TrimRight(rootPath, "/")
	user := strings.TrimLeft(userPath, "/")
	if root == "" || root == "/" {
		return "/" + user
	}
	if user == "" {
		return root
	}
	return root + "/" + user
}

// LoadConfigAuto loads config from path or auto-discovers it.
func LoadConfigAuto(path string) (configPath string, cfg *Config, vr *ValidationResult, err error) {
	if path == "" {
		path = FindConfigFile()
	}
	if path == "" {
		return "", nil, nil, fmt.Errorf("no config file found")
	}
	cfg, vr, err = LoadConfig(path)
	if err != nil {
		return path, nil, nil, err
	}
	return path, cfg, vr, nil
}

// FindMount finds a mount by name; if name is empty, returns the default mount.
func FindMount(cfg *Config, name string) *MountInstance {
	if name != "" {
		for _, m := range cfg.Mounts {
			if m.Name == name {
				return &m
			}
		}
		return nil
	}
	return FindDefaultMount(cfg)
}

// MakeCipher creates an RcloneCipher from encryption config with optional overrides.
func MakeCipher(enc EncryptionConfig, defaults EncryptionConfig, password, salt string) (*cipher.RcloneCipher, error) {
	if password == "" {
		password = enc.Password
	}
	if password == "" {
		password = defaults.Password
	}
	if password == "" {
		return nil, fmt.Errorf("encryption password required")
	}

	if salt == "" {
		salt = enc.Salt
	}
	if salt == "" {
		salt = defaults.Salt
	}

	filenameEnc := enc.FileNameEncoding
	if filenameEnc == "" {
		filenameEnc = defaults.FileNameEncoding
	}
	if filenameEnc == "" {
		filenameEnc = "base32"
	}
	filenameEncryption := enc.FileNameEncryption
	if filenameEncryption == "" {
		filenameEncryption = defaults.FileNameEncryption
	}
	if filenameEncryption == "" {
		filenameEncryption = "standard"
	}

	return cipher.NewRcloneCipher(password, salt, filenameEnc, filenameEncryption)
}

