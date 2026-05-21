package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
	factory "github.com/yinzhenyu/skills/qrypt/internal/drive/factory"
	"github.com/yinzhenyu/skills/qrypt/internal/drive/localfs"
	quark "github.com/yinzhenyu/skills/qrypt/internal/drive/quark"
	"github.com/yinzhenyu/skills/qrypt/internal/log"
)

type pathResolver interface {
	ResolvePath(ctx context.Context, path string) (string, error)
}

// ParseMountPath parses "mount_name:path" format.
// Returns (mountName, path). If no prefix, mountName is empty.
func ParseMountPath(s string) (mountName, path string) {
	if s == "" || s[0] == '.' || s[0] == '/' || s[0] == '~' {
		return "", s
	}
	idx := strings.Index(s, ":/")
	if idx <= 0 || idx > 32 {
		return "", s
	}
	candidate := s[:idx]
	if !config.ValidMountName(candidate) {
		return "", s
	}
	return candidate, s[idx+1:]
}

// pickMount selects the mount instance by name, falling back to the single mount.
func pickMount(cfg *config.Config, name string) *config.MountInstance {
	if name != "" {
		for _, m := range cfg.Mounts {
			if m.Name == name {
				return &m
			}
		}
		return nil
	}
	if len(cfg.Mounts) == 1 {
		return &cfg.Mounts[0]
	}
	if len(cfg.Mounts) > 1 {
		return nil
	}
	return nil
}

func loadToolCfg(cmd *cobra.Command) (*config.Config, *crypt.RcloneCipher) {
	cfg := loadToolCfgOnly(cmd)

	pwd, _ := cmd.Flags().GetString("password")
	salt, _ := cmd.Flags().GetString("salt")

	// Determine password/encoding/encryption from command flag, first mount, or defaults
	encPass := pwd
	filenameEnc := ""
	filenameEncryption := ""
	if len(cfg.Mounts) > 0 {
		rc := cfg.MergeInstanceConfig(cfg.Mounts[0])
		if encPass == "" {
			encPass = rc.Encryption.Password
		}
		filenameEnc = rc.Encryption.FileNameEncoding
		filenameEncryption = rc.Encryption.FileNameEncryption
	}
	if encPass == "" {
		encPass = cfg.Defaults.Encryption.Password
	}
	if filenameEnc == "" {
		filenameEnc = cfg.Defaults.Encryption.FileNameEncoding
	}
	if filenameEnc == "" {
		filenameEnc = "base32"
	}
	if filenameEncryption == "" {
		filenameEncryption = cfg.Defaults.Encryption.FileNameEncryption
	}
	if filenameEncryption == "" {
		filenameEncryption = "standard"
	}

	if encPass == "" {
		fmt.Println("错误: 缺少加密密码 (配置文件或 --password 参数)")
		os.Exit(1)
	}
	if pwd != "" {
		cfg.Defaults.Encryption.Password = pwd
	}

	cipher, err := crypt.NewRcloneCipher(encPass, salt, filenameEnc, filenameEncryption)
	if err != nil {
		fmt.Printf("加密引擎初始化失败: %v\n", err)
		os.Exit(1)
	}
	return cfg, cipher
}

func loadToolCfgOnly(cmd *cobra.Command) *config.Config {
	configPath, _ := cmd.Flags().GetString("config")
	explicitConfig := configPath != ""
	if configPath == "" {
		configPath = config.FindConfigFile()
	}

	cfg, vr, err := config.LoadConfig(configPath)
	if err != nil {
		if explicitConfig {
			fmt.Printf("加载配置文件失败: %v\n", err)
			os.Exit(1)
		}
		cfg = config.DefaultConfig()
		vr = config.ValidateConfig(cfg)
	}

	if vr != nil && !vr.Valid {
		var hasError bool
		for _, c := range vr.Checks {
			if c.Status == "error" {
				if !hasError {
					fmt.Fprintf(os.Stderr, "配置文件校验失败:\n")
					hasError = true
				}
				fmt.Fprintf(os.Stderr, "  [%s] %s\n", c.Field, c.Message)
			}
		}
	}

	logFile := config.ExpandHome(cfg.Log.File)
	if logger, lerr := log.New(cfg.Log.Level, logFile, nil); lerr == nil {
		log.L = logger
	}
	return cfg
}

// resolveMount returns the mount name from --flag or path prefix.
func resolveMount(cmd *cobra.Command, path *string) string {
	// Check --mount flag first
	if mountName, _ := cmd.Flags().GetString("mount"); mountName != "" {
		return mountName
	}
	// Check path prefix
	if path != nil {
		mountName, cleanPath := ParseMountPath(*path)
		if mountName != "" {
			*path = cleanPath
			return mountName
		}
	}
	return ""
}

func loadToolDriver(cfg *config.Config, cipher *crypt.RcloneCipher) drive.Driver {
	return loadToolDriverForMount(cfg, cipher, "")
}

func loadToolDriverForMount(cfg *config.Config, globalCipher *crypt.RcloneCipher, mountName string) drive.Driver {
	m := pickMount(cfg, mountName)
	if m == nil {
		if mountName != "" {
			fmt.Printf("挂载实例 %q 未找到\n", mountName)
		} else {
			fmt.Printf("未指定挂载实例 (配置中有 %d 个，使用 --mount 或 mount_name:path 选择)\n", len(cfg.Mounts))
		}
		os.Exit(1)
	}

	rc := cfg.MergeInstanceConfig(*m)
	drv, err := factory.NewDriverFromType(rc.Type, rc.Params)
	if err != nil {
		fmt.Printf("创建驱动失败: %v\n", err)
		os.Exit(1)
	}
	if err := drv.Init(context.Background()); err != nil {
		fmt.Printf("认证失败: %v\n", err)
		os.Exit(1)
	}

	// Use mount-level encryption if available; fall back to global cipher
	mountCipher := globalCipher
	if rc.Encryption.Password != "" {
		var cerr error
		mountCipher, cerr = crypt.NewRcloneCipher(rc.Encryption.Password, rc.Encryption.Salt, rc.Encryption.FileNameEncoding, rc.Encryption.FileNameEncryption)
		if cerr != nil {
			fmt.Printf("加密引擎初始化失败: %v\n", cerr)
			os.Exit(1)
		}
	}
	switch d := drv.(type) {
	case *quark.QuarkDriver:
		if mountCipher != nil {
			d.SetCipher(mountCipher)
		}
	case *localfs.LocalDriver:
		if mountCipher != nil {
			d.SetCipher(mountCipher)
		}
	}
	return drv
}

func maskStr(s string) string {
	if s == "" {
		return "(未设置)"
	}
	if len(s) <= 4 {
		return "****"
	}
	return s[:1] + "****" + s[len(s)-1:]
}

func resolveFullPath(rootPath, userPath string) string {
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

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n/div >= unit && exp < len("KMGTPE")-1 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func formatComma(n int) string {
	s := fmt.Sprintf("%d", n)
	parts := make([]string, 0)
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return strings.Join(parts, ",")
}

func percentOrZero(a, b int64) int {
	if b == 0 {
		return 0
	}
	return int(a * 100 / b)
}
