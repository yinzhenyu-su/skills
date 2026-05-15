package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func runConfig(cmd *cobra.Command, args []string) {
	cfg, _ := loadToolCfg(cmd)
	fmt.Println("=== Qrypt 配置 ===")
	fmt.Printf("驱动类型:     %s\n", cfg.Drive.Type)
	fmt.Printf("根路径: %s\n", cfg.RootPath())
	fmt.Printf("缓存目录:     %s\n", cfg.Cache.Dir)
	fmt.Printf("挂载点:       %s\n", cfg.Mount.Point)
	fmt.Printf("加密密码:     %s\n", maskStr(cfg.Encryption.Password))
	if cfg.Encryption.Salt != "" {
		fmt.Printf("加密盐:       %s\n", cfg.Encryption.Salt)
	}
	fmt.Printf("日志级别:     %s\n", cfg.Log.Level)
	fmt.Printf("并发上传:     %d\n", cfg.Sync.ConcurrentUploads)
	fmt.Printf("缓存上限:     %s\n", cfg.Cache.MaxSize)
}
