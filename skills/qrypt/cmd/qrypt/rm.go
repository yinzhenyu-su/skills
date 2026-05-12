package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/quark"
)

func runRm(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)
	quarkClient := quark.NewClient(cfg.Quark.Cookie)
	cacheSvc := quark.NewCacheService()
	fileSvc := quark.NewFileService(quarkClient, cacheSvc, cipher)
	manageSvc := quark.NewManageService(quarkClient)

	if err := fileSvc.Auth(); err != nil {
		fmt.Printf("认证失败: %v\n", err)
		os.Exit(1)
	}

	path := args[0]
	fullPath := resolveFullPath(cfg.Quark.RootPath, path)

	fid, err := fileSvc.ResolvePath(fullPath)
	if err != nil {
		fmt.Printf("无法解析路径: %v\n", err)
		os.Exit(1)
	}

	if err := manageSvc.Delete([]string{fid}); err != nil {
		fmt.Printf("删除失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("已删除: %s\n", path)
}
