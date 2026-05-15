package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	// Deprecated: Use Drive.Quark instead. Kept for backward compatibility.
	Quark      QuarkConfig      `toml:"quark"`
	Drive      DriveConfig      `toml:"drive"`
	Encryption EncryptionConfig `toml:"encryption"`
	Cache      CacheConfig      `toml:"cache"`
	Mount      MountConfig      `toml:"mount"`
	Sync       SyncConfig       `toml:"sync"`
	Log        LogConfig        `toml:"log"`
}

// QuarkConfig is the deprecated top-level Quark configuration.
// Deprecated: Use Drive.Quark instead.
type QuarkConfig struct {
	Cookie   string `toml:"cookie"`
	RootPath string `toml:"root_path"`
}

// DriveConfig selects the storage backend and holds driver-specific options.
type DriveConfig struct {
	Type   string          `toml:"type"` // "quark" | "yun139"
	Quark  *QuarkOptions   `toml:"quark"`
	Yun139 *Yun139Options  `toml:"yun139"`
}

// QuarkOptions holds configuration for the Quark drive backend.
type QuarkOptions struct {
	Cookie   string `toml:"cookie"`
	RootPath string `toml:"root_path"`
}

// Yun139Options holds configuration for the 139 cloud drive backend.
type Yun139Options struct {
	Authorization string `toml:"authorization"`
	RootID        string `toml:"root_id"`
}

type EncryptionConfig struct {
	Password string `toml:"password"`
	Salt     string `toml:"salt"`
}

type CacheConfig struct {
	Dir            string `toml:"dir"`
	MaxSize        string `toml:"max_size"`
	MemCacheSizeMB int    `toml:"mem_cache_size_mb"`
}

type MountConfig struct {
	Point      string `toml:"point"`
	AllowOther bool   `toml:"allow_other"`
}

type SyncConfig struct {
	MaxRetries        int    `toml:"max_retries"`
	ConcurrentUploads int    `toml:"concurrent_uploads"`
	DirCacheTTL       string `toml:"dir_cache_ttl"`
}

type LogConfig struct {
	Level      string `toml:"level"`
	File       string `toml:"file"`
	MaxSize    int    `toml:"max_size"`
	MaxBackups int    `toml:"max_backups"`
	MaxAge     int    `toml:"max_age"`
	Compress   *bool  `toml:"compress"`
}

func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	return &Config{
		Quark: QuarkConfig{
			RootPath: "/Test",
		},
		Drive: DriveConfig{
			Type: "quark",
			Quark: &QuarkOptions{
				RootPath: "/Test",
			},
		},
		Encryption: EncryptionConfig{},
		Cache: CacheConfig{
			Dir:     filepath.Join(homeDir, ".qrypt", "cache"),
			MaxSize: "10GB",
		},
		Mount: MountConfig{
			Point:     filepath.Join(homeDir, "Qrypt"),
			AllowOther: false,
		},
		Sync: SyncConfig{
			MaxRetries:        3,
			ConcurrentUploads: 3,
			DirCacheTTL:       "5m",
		},
		Log: LogConfig{
			Level: "debug",
			File:  filepath.Join(homeDir, ".qrypt", "qrypt.log"),
		},
	}
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

func LoadConfig(path string) (*Config, error) {
	config := DefaultConfig()
	if path == "" {
		return config, nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", path)
	}
	if _, err := toml.DecodeFile(path, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Backward compatibility: auto-migrate old [quark] section to DriveConfig.
	if config.Drive.Type == "" && config.Quark.Cookie != "" {
		config.Drive.Type = "quark"
		config.Drive.Quark = &QuarkOptions{
			Cookie:   config.Quark.Cookie,
			RootPath: config.Quark.RootPath,
		}
	}
	// If new format was used, propagate back to old field for CLI tool compat.
	if config.Drive.Quark != nil && config.Quark.Cookie == "" {
		config.Quark.Cookie = config.Drive.Quark.Cookie
		config.Quark.RootPath = config.Drive.Quark.RootPath
	}

	config.Cache.Dir = ExpandHome(config.Cache.Dir)
	config.Mount.Point = ExpandHome(config.Mount.Point)
	config.Log.File = ExpandHome(config.Log.File)
	return config, nil
}

func FindConfigFile() string {
	candidates := []string{"qrypt.toml"}
	if homeDir, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates,
			filepath.Join(homeDir, ".config", "qrypt", "qrypt.toml"),
			filepath.Join(homeDir, ".qrypt.toml"),
		)
	}
	candidates = append(candidates, "/etc/qrypt/qrypt.toml")
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// RootPath returns the root path/ID for the currently configured driver type.
// This is used by CLI tools (ls/cat/rm/mv/push/pull/find) to resolve user-provided
// paths relative to the configured root. Returns "/" when nothing is configured.
func (c *Config) RootPath() string {
	switch c.Drive.Type {
	case "quark":
		if c.Drive.Quark != nil {
			return c.Drive.Quark.RootPath
		}
	case "yun139":
		if c.Drive.Yun139 != nil {
			return c.Drive.Yun139.RootID
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
