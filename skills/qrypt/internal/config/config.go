package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/BurntSushi/toml"
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

	// Mount instances.
	Mounts   []MountInstance `toml:"mounts"`
	Defaults DefaultsConfig  `toml:"defaults"`

	// Shared config (not per-mount).
	Log LogConfig `toml:"log"`

	// Legacy fields (no TOML tags — no longer read from config files).
	// Kept as Go fields for internal code that still references them.
	Quark QuarkConfig
	Drive DriveConfig
	Mount MountConfig

	Encryption EncryptionConfig
	Cache      CacheConfig
	Sync       SyncConfig
}

// DefaultsConfig holds global default values inherited by each mount instance.
type DefaultsConfig struct {
	Encryption EncryptionConfig `toml:"encryption"`
	Sync       SyncConfig       `toml:"sync"`
	Cache      CacheConfig      `toml:"cache"`
}

// MountInstance declares one cloud drive mount.
type MountInstance struct {
	Name       string      `toml:"name"`        // unique identifier, [a-z0-9-]{1,32}
	Type       string      `toml:"type"`        // "quark" | "yun139" | "localfs"
	MountPoint string      `toml:"mount_point"` // FUSE mount path
	Enabled    *bool       `toml:"enabled"`     // nil = true
	AllowOther bool        `toml:"allow_other"`

	Params     MountParams        `toml:"params"`
	Encryption *EncryptionConfig  `toml:"encryption"` // nil = use defaults
	Sync       *SyncConfig        `toml:"sync"`       // nil = use defaults
	Cache      *CacheConfig       `toml:"cache"`      // nil = use defaults
}

// MountParams holds driver-specific configuration parameters.
type MountParams struct {
	// quark
	Cookie   string `toml:"cookie"`
	RootPath string `toml:"root_path"`

	// yun139
	Authorization string `toml:"authorization"`
	RootID        string `toml:"root_id"`

	// localfs
	LocalRoot string `toml:"local_root"`
}

// ResolvedMountConfig is a mount instance with all defaults merged in.
type ResolvedMountConfig struct {
	Name       string
	Type       string
	MountPoint string
	AllowOther bool
	Enabled    bool
	Params     MountParams

	Encryption EncryptionConfig
	Sync       SyncConfig
	Cache      CacheConfig
	CacheDir   string // computed: ~/.qrypt/cache/<name>/
}

// Legacy types (no TOML tags — kept for internal code).
type QuarkConfig struct {
	Cookie   string
	RootPath string
}

type LocalFSOptions struct {
	RootPath string
}

type DriveConfig struct {
	Type    string
	Quark   *QuarkOptions
	Yun139  *Yun139Options
	LocalFS *LocalFSOptions
}

type QuarkOptions struct {
	Cookie   string
	RootPath string
}

type Yun139Options struct {
	Authorization string
	RootID        string
}

type EncryptionConfig struct {
	Password         string `toml:"password"`
	Salt             string `toml:"salt"`
	FileNameEncoding string `toml:"filename_encoding"`
}

type CacheConfig struct {
	Dir            string `toml:"-"`      // computed: $QRYPT_WORK_DIR/cache
	MaxSize        string `toml:"max_size"`
	MemCacheSizeMB int    `toml:"mem_cache_size_mb"`
}

type MountConfig struct {
	Point      string
	AllowOther bool
}

type SyncConfig struct {
	MaxRetries        int    `toml:"max_retries"`
	ConcurrentUploads int    `toml:"concurrent_uploads"`
	DirCacheTTL       string `toml:"dir_cache_ttl"`
}

type LogConfig struct {
	Level      string `toml:"level"`
	File       string `toml:"-"`      // computed: $QRYPT_WORK_DIR/qrypt.log
	MaxSize    int    `toml:"max_size"`
	MaxBackups int    `toml:"max_backups"`
	MaxAge     int    `toml:"max_age"`
	Compress   *bool  `toml:"compress"`
}

func DefaultConfig() *Config {
	workDir := WorkDir()
	return &Config{
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
			},
			Cache: CacheConfig{
				MaxSize: "10GB",
			},
		},
	}
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

	cacheDir := filepath.Join(WorkDir(), "cache", m.Name)

	return &ResolvedMountConfig{
		Name:       m.Name,
		Type:       m.Type,
		MountPoint: ExpandHome(m.MountPoint),
		AllowOther: m.AllowOther,
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

	workDir := WorkDir()
	config.Log.File = filepath.Join(workDir, "qrypt.log")

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

// RootPath returns the root path/ID for the first mount instance.
func (c *Config) RootPath() string {
	if len(c.Mounts) > 0 {
		return RootPathForMount(c.Mounts[0])
	}
	return "/"
}

// RootPathForMount returns the root path/ID for a specific mount instance.
func RootPathForMount(m MountInstance) string {
	switch m.Type {
	case "quark":
		if m.Params.RootPath != "" {
			return m.Params.RootPath
		}
	case "yun139":
		if m.Params.RootID != "" {
			return m.Params.RootID
		}
	case "localfs":
		if m.Params.LocalRoot != "" {
			return m.Params.LocalRoot
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
