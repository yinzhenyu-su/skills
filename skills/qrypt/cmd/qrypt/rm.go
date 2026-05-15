package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
)

func runRm(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)
	drv := loadToolDriver(cfg, cipher)

	path := args[0]
	fullPath := resolveFullPath(cfg.RootPath(), path)

	if fullPath == "/" {
		fmt.Printf("错误: 无法删除根目录\n")
		os.Exit(1)
	}

	recursive, _ := cmd.Flags().GetBool("recursive")
	recursiveUpper, _ := cmd.Flags().GetBool("recursive-upper")
	isRecursive := recursive || recursiveUpper
	force, _ := cmd.Flags().GetBool("force")
	interactive, _ := cmd.Flags().GetBool("interactive")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	resolver, ok := drv.(pathResolver)
	if !ok {
		fmt.Printf("该驱动不支持路径解析\n")
		os.Exit(1)
	}

	parentPath := filepath.Dir(fullPath)
	baseName := filepath.Base(fullPath)

	parentFid, err := resolver.ResolvePath(context.Background(), parentPath)
	if err != nil {
		if force {
			os.Exit(0)
		}
		fmt.Printf("无法解析父路径: %v\n", err)
		os.Exit(1)
	}

	entries, err := drv.List(context.Background(), parentFid)
	if err != nil {
		if force {
			os.Exit(0)
		}
		fmt.Printf("无法列出目录内容: %v\n", err)
		os.Exit(1)
	}

	var targetEntry drive.Entry
	found := false
	encSeg := ""
	if cipher != nil {
		encSeg = cipher.EncryptSegment(baseName)
	}
	for _, e := range entries {
		if e.Name == baseName || (encSeg != "" && strings.EqualFold(e.Name, encSeg)) {
			targetEntry = e
			found = true
			break
		}
	}

	if !found {
		if force {
			os.Exit(0)
		}
		fmt.Printf("文件不存在: %s\n", path)
		os.Exit(1)
	}

	if targetEntry.IsDir && !isRecursive {
		fmt.Printf("错误: %s 是一个目录。请使用 -r 或 -R 递归删除。\n", path)
		os.Exit(1)
	}

	if interactive {
		fmt.Printf("确认删除 %s? (y/N): ", path)
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Printf("已取消删除: %s\n", path)
			return
		}
	}

	if dryRun {
		fmt.Printf("[Dry Run] 将要删除: %s (fid: %s)\n", path, targetEntry.ID)
		return
	}

	w, ok := drv.(drive.Writer)
	if !ok {
		fmt.Printf("该驱动不支持删除操作\n")
		os.Exit(1)
	}

	if err := w.Remove(context.Background(), targetEntry); err != nil {
		fmt.Printf("删除失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("已删除: %s\n", path)
}
