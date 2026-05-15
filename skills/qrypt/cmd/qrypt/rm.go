package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
)

func runRm(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)
	drv := loadToolDriver(cfg, cipher)

	path := args[0]
	fullPath := resolveFullPath(cfg.RootPath(), path)

	resolver, ok := drv.(pathResolver)
	if !ok {
		fmt.Printf("该驱动不支持路径解析\n")
		os.Exit(1)
	}
	fid, err := resolver.ResolvePath(context.Background(), fullPath)
	if err != nil {
		fmt.Printf("无法解析路径: %v\n", err)
		os.Exit(1)
	}

	w, ok := drv.(drive.Writer)
	if !ok {
		fmt.Printf("该驱动不支持删除操作\n")
		os.Exit(1)
	}

	if err := w.Remove(context.Background(), drive.Entry{ID: fid}); err != nil {
		fmt.Printf("删除失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("已删除: %s\n", path)
}
