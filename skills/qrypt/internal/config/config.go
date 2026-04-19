// Package config 处理 qrypt 配置文件
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// Config 是 qrypt 的主配置结构
type Config struct {
	Quark      QuarkConfig      `toml:"quark"`
	Encryption EncryptionConfig `toml:"encryption"`
	Cache      CacheConfig      `toml:"cache"`
	Mount      MountConfig      `toml:"mount"`
	Sync       SyncConfig       `toml:"sync"`
	Log        LogConfig        `toml:"log"`
}

// QuarkConfig 夸克网盘相关配置
type QuarkConfig struct {
	Cookie   string `toml:"cookie"`
	RootPath string `toml:"root_path"`
}

// EncryptionConfig 加密相关配置
type EncryptionConfig struct {
	Password string `toml:"password"`
	Salt     string `toml:"salt"`
}

// CacheConfig 缓存相关配置
type CacheConfig struct {
	Dir     string `toml:"dir"`
	DBName  string `toml:"db_name"`
	MaxSize string `toml:"max_size"` // 如 "10GB", "500MB"
}

// MountConfig 挂载相关配置
type MountConfig struct {
	Point     string `toml:"point"`
	AllowOther bool  `toml:"allow_other"`
}

// SyncConfig 同步相关配置
type SyncConfig struct {
	MaxRetries        int    `toml:"max_retries"`
	ConcurrentUploads int    `toml:"concurrent_uploads"`
	DirCacheTTL       string `toml:"dir_cache_ttl"` // 如 "5m", "30s"
}

// LogConfig 日志相关配置
type LogConfig struct {
	Level string `toml:"level"` // debug, info, warn, error
	File  string `toml:"file"`  // 日志文件路径，空则输出到 stdout
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	return &Config{
		Quark: QuarkConfig{
			RootPath: "/",
		},
		Encryption: EncryptionConfig{},
		Cache: CacheConfig{
			Dir:     filepath.Join(homeDir, ".qrypt", "cache"),
			DBName:  "qrypt_cache.db",
			MaxSize: "10GB",
		},
		Mount: MountConfig{
			AllowOther: false,
		},
		Sync: SyncConfig{
			MaxRetries:        3,
			ConcurrentUploads: 3,
			DirCacheTTL:       "5m",
		},
		Log: LogConfig{
			Level: "info",
		},
	}
}

// ExpandHome 将路径中的 ~ 展开为用户家目录
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
	return filepath.Join(homeDir, path[2:]) // skip "~/"
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (*Config, error) {
	config := DefaultConfig()

	if path == "" {
		return config, nil
	}

	// 检查文件是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", path)
	}

	// 解析 TOML 文件
	if _, err := toml.DecodeFile(path, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// 展开所有路径中的 ~
	config.Cache.Dir = ExpandHome(config.Cache.Dir)
	config.Cache.DBName = ExpandHome(config.Cache.DBName)
	config.Mount.Point = ExpandHome(config.Mount.Point)
	config.Log.File = ExpandHome(config.Log.File)

	return config, nil
}

// FindConfigFile 在常见位置查找配置文件
func FindConfigFile() string {
	// 搜索顺序：
	// 1. 当前目录 ./qrypt.toml
	// 2. ~/.config/qrypt/qrypt.toml
	// 3. ~/.qrypt.toml
	// 4. /etc/qrypt/qrypt.toml

	candidates := []string{
		"qrypt.toml",
	}

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

// ParseSize 解析大小字符串（如 "10GB", "500MB"）返回字节数
func ParseSize(s string) (int64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}

	var multiplier int64 = 1
	var numStr string

	// 检查后缀
	suffixes := map[string]int64{
		"KB": 1024,
		"MB": 1024 * 1024,
		"GB": 1024 * 1024 * 1024,
		"TB": 1024 * 1024 * 1024 * 1024,
		"K":  1024,
		"M":  1024 * 1024,
		"G":  1024 * 1024 * 1024,
		"T":  1024 * 1024 * 1024 * 1024,
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

// ParseDuration 解析时间字符串（如 "5m", "30s"）
func ParseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty duration string")
	}
	return time.ParseDuration(s)
}
