package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/daemon"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

func runRm(cmd *cobra.Command, args []string) {
	socketPath := daemon.FindSocketPath()
	if daemon.IsDaemonRunning(socketPath) {
		runRmViaDaemon(cmd, args, socketPath)
	} else {
		runRmDirect(cmd, args)
	}
}

func runRmViaDaemon(cmd *cobra.Command, args []string, socketPath string) {
	path := args[0]
	mountName := resolveMount(cmd, &path)
	recursive, _ := cmd.Flags().GetBool("recursive")
	recursiveUpper, _ := cmd.Flags().GetBool("recursive-upper")
	isRecursive := recursive || recursiveUpper
	force, _ := cmd.Flags().GetBool("force")
	interactive, _ := cmd.Flags().GetBool("interactive")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	password, _ := cmd.Flags().GetString("password")
	salt, _ := cmd.Flags().GetString("salt")

	client, err := daemon.DialWS(socketPath)
	if err != nil {
		fmt.Printf("无法连接到 qryptd: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	for _, p := range args {
		path = p
		mountName = resolveMount(cmd, &path)

		if interactive {
			fmt.Printf("确认删除 %s? (y/N): ", path)
			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))
			if response != "y" && response != "yes" {
				fmt.Printf("已取消删除: %s\n", path)
				continue
			}
		}

		if dryRun {
			fmt.Printf("[Dry Run] 将要删除: %s\n", path)
			continue
		}

		resp, rpcErr := client.Call("remove", protocol.RemoveParams{
			MountName: mountName,
			Path:      path,
			Recursive: isRecursive,
			Force:     force,
			Password:  password,
			Salt:      salt,
		})
		if rpcErr != nil {
			fmt.Printf("RPC 错误: %v\n", rpcErr)
			os.Exit(1)
		}
		if resp.Error != nil {
			fmt.Printf("删除失败 (%s): %s\n", path, resp.Error.Message)
			os.Exit(1)
		}
		fmt.Printf("已删除: %s\n", path)
	}
}

func runRmDirect(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)
	path := args[0]
	mountName := resolveMount(cmd, &path)
	args[0] = path
	drv := loadToolDriverForMount(cfg, cipher, mountName)

	recursive, _ := cmd.Flags().GetBool("recursive")
	recursiveUpper, _ := cmd.Flags().GetBool("recursive-upper")
	isRecursive := recursive || recursiveUpper
	force, _ := cmd.Flags().GetBool("force")
	interactive, _ := cmd.Flags().GetBool("interactive")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	resolver, ok := drv.(drive.PathResolver)
	if !ok {
		fmt.Printf("该驱动不支持路径解析\n")
		os.Exit(1)
	}

	w, wOk := drv.(drive.Writer)

	var exitCode int
	for _, p := range args {
		fullPath := config.ResolveFullPath(cfg.RootPath(), p)

		if fullPath == "/" {
			fmt.Printf("错误: 无法删除根目录\n")
			exitCode = 1
			continue
		}

		parentPath := filepath.Dir(fullPath)
		baseName := filepath.Base(fullPath)

		parentFid, err := resolver.ResolvePath(context.Background(), parentPath)
		if err != nil {
			if !force {
				fmt.Printf("无法解析父路径: %s: %v\n", p, err)
				exitCode = 1
			}
			continue
		}

		entries, err := drv.List(context.Background(), parentFid)
		if err != nil {
			if !force {
				fmt.Printf("无法列出目录内容: %s: %v\n", p, err)
				exitCode = 1
			}
			continue
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
			if !force {
				fmt.Printf("文件不存在: %s\n", p)
				exitCode = 1
			}
			continue
		}

		if targetEntry.IsDir && !isRecursive {
			fmt.Printf("错误: %s 是一个目录。请使用 -r 或 -R 递归删除。\n", p)
			exitCode = 1
			continue
		}

		if interactive {
			fmt.Printf("确认删除 %s? (y/N): ", p)
			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))
			if response != "y" && response != "yes" {
				fmt.Printf("已取消删除: %s\n", p)
				continue
			}
		}

		if dryRun {
			fmt.Printf("[Dry Run] 将要删除: %s (fid: %s)\n", p, targetEntry.ID)
			continue
		}

		if !wOk {
			fmt.Printf("该驱动不支持删除操作\n")
			exitCode = 1
			continue
		}

		if err := w.Remove(context.Background(), targetEntry); err != nil {
			fmt.Printf("删除失败: %s: %v\n", p, err)
			exitCode = 1
			continue
		}

		fmt.Printf("已删除: %s\n", p)
	}

	if exitCode != 0 {
		os.Exit(exitCode)
	}
}
