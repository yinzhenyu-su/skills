package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

func runConfig(cmd *cobra.Command, args []string) {
	cfg, _ := loadToolCfg(cmd)
	fmt.Println("=== Qrypt 配置 ===")
	fmt.Printf("版本:         %s\n", cfg.Version)
	fmt.Printf("工作目录:     %s\n", config.WorkDir())
	fmt.Printf("日志级别:     %s\n", cfg.Log.Level)
	fmt.Printf("日志文件:     %s\n", cfg.Log.File)
	fmt.Println()

	if len(cfg.Mounts) == 0 {
		fmt.Println("未配置挂载实例")
		return
	}

	for _, m := range cfg.Mounts {
		rc := cfg.MergeInstanceConfig(m)
		fmt.Printf("── %s (%s) ──\n", rc.Name, rc.Type)
		fmt.Printf("  挂载点:     %s\n", rc.MountPoint)
		fmt.Printf("  根路径:     %s\n", config.RootPathForMount(m))
		fmt.Printf("  缓存目录:   %s\n", rc.CacheDir)
		fmt.Printf("  缓存上限:   %s\n", rc.Cache.MaxSize)
		fmt.Printf("  加密密码:   %s\n", maskStr(rc.Encryption.Password))
		if rc.Encryption.Salt != "" {
			fmt.Printf("  加密盐:     %s\n", rc.Encryption.Salt)
		}
		fmt.Printf("  并发上传:   %d\n", rc.Sync.ConcurrentUploads)
		fmt.Println()
	}
}
