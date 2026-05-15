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
	quark "github.com/yinzhenyu/skills/qrypt/internal/drive/quark"
	"github.com/yinzhenyu/skills/qrypt/internal/log"
)

type pathResolver interface {
	ResolvePath(ctx context.Context, path string) (string, error)
}

func loadToolCfg(cmd *cobra.Command) (*config.Config, *crypt.RcloneCipher) {
	configPath, _ := cmd.Flags().GetString("config")
	explicitConfig := configPath != ""
	if configPath == "" {
		configPath = config.FindConfigFile()
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		if explicitConfig {
			fmt.Printf("加载配置文件失败: %v\n", err)
			os.Exit(1)
		}
		cfg = config.DefaultConfig()
	}

	logFile := config.ExpandHome(cfg.Log.File)
	if logger, lerr := log.New(cfg.Log.Level, logFile, nil); lerr == nil {
		log.L = logger
	}

	if pwd, _ := cmd.Flags().GetString("password"); pwd != "" {
		cfg.Encryption.Password = pwd
	}
	if salt, _ := cmd.Flags().GetString("salt"); salt != "" {
		cfg.Encryption.Salt = salt
	}
	if cfg.Encryption.Password == "" {
		fmt.Println("错误: 缺少加密密码 (配置文件或 --password 参数)")
		os.Exit(1)
	}

	cipher, err := crypt.NewRcloneCipher(cfg.Encryption.Password, cfg.Encryption.Salt)
	if err != nil {
		fmt.Printf("加密引擎初始化失败: %v\n", err)
		os.Exit(1)
	}
	return cfg, cipher
}

func loadToolDriver(cfg *config.Config, cipher *crypt.RcloneCipher) drive.Driver {
	drv, err := factory.NewDriverFromConfig(cfg.Drive)
	if err != nil {
		fmt.Printf("创建驱动失败: %v\n", err)
		os.Exit(1)
	}
	if err := drv.Init(context.Background()); err != nil {
		fmt.Printf("认证失败: %v\n", err)
		os.Exit(1)
	}
	if qd, ok := drv.(*quark.QuarkDriver); ok && cipher != nil {
		qd.SetCipher(cipher)
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
