package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/daemon"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

func runMkdir(cmd *cobra.Command, args []string) {
	socketPath := daemon.FindSocketPath()
	if daemon.IsDaemonRunning(socketPath) {
		runMkdirViaDaemon(cmd, args, socketPath)
	} else {
		runMkdirDirect(cmd, args)
	}
}

func runMkdirViaDaemon(cmd *cobra.Command, args []string, socketPath string) {
	path := args[0]
	mountName := resolveMount(cmd, &path)
	parents, _ := cmd.Flags().GetBool("parents")
	password, _ := cmd.Flags().GetString("password")
	salt, _ := cmd.Flags().GetString("salt")

	client, err := daemon.DialClient(socketPath)
	if err != nil {
		fmt.Printf("无法连接到 qryptd: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	resp, rpcErr := client.Call("mkdir", protocol.MkdirParams{
		MountName: mountName,
		Path:      path,
		Parents:   parents,
		Password:  password,
		Salt:      salt,
	})
	if rpcErr != nil {
		fmt.Printf("RPC 错误: %v\n", rpcErr)
		os.Exit(1)
	}
	if resp.Error != nil {
		fmt.Printf("创建目录失败: %s\n", resp.Error.Message)
		os.Exit(1)
	}
	fmt.Printf("已创建: %s\n", path)
}

func runMkdirDirect(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)
	path := args[0]
	mountName := resolveMount(cmd, &path)
	args[0] = path
	drv := loadToolDriverForMount(cfg, cipher, mountName)

	parents, _ := cmd.Flags().GetBool("parents")

	w, ok := drv.(drive.Writer)
	if !ok {
		fmt.Println("当前驱动不支持写入操作")
		os.Exit(1)
	}

	rootPath := cfg.RootPath()

	for _, userPath := range args {
		fullPath := config.ResolveFullPath(rootPath, userPath)
		err := createDirectory(context.Background(), drv, w, cipher, fullPath, userPath, parents)
		if err != nil {
			fmt.Printf("创建目录失败: %s: %v\n", userPath, err)
			os.Exit(1)
		}
	}
}

func createDirectory(ctx context.Context, drv drive.Driver, w drive.Writer, cipher *crypt.RcloneCipher, fullPath, userPath string, parents bool) error {
	currentFid := "0"
	if !parents {
		segments := strings.Split(strings.Trim(fullPath, "/"), "/")
		if len(segments) > 1 {
			parentPath := filepath.Dir("/" + strings.Trim(fullPath, "/"))
			if resolver, ok := drv.(drive.PathResolver); ok {
				var err error
				currentFid, err = resolver.ResolvePath(ctx, parentPath)
				if err != nil {
					return fmt.Errorf("父目录不存在: %v", err)
				}
			} else {
				return fmt.Errorf("不支持路径解析")
			}
		}
		targetName := segments[len(segments)-1]
		if targetName == "" {
			return nil
		}
		entries, err := drv.List(ctx, currentFid)
		if err != nil {
			return err
		}
		encName := targetName
		if cipher != nil {
			encName = cipher.EncryptSegment(targetName)
		}
		for _, e := range entries {
			if e.Name == targetName || strings.EqualFold(e.Name, encName) {
				return fmt.Errorf("文件或目录已存在")
			}
		}
		_, err = w.Mkdir(ctx, currentFid, encName)
		return err
	}

	userRel := strings.TrimLeft(userPath, "/")
	segments := strings.Split(userRel, "/")
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		entries, err := drv.List(ctx, currentFid)
		if err != nil {
			return fmt.Errorf("列出目录内容失败: %w", err)
		}
		encSeg := seg
		if cipher != nil {
			encSeg = cipher.EncryptSegment(seg)
		}
		found := false
		for _, e := range entries {
			if e.Name == seg || strings.EqualFold(e.Name, encSeg) {
				currentFid = e.ID
				found = true
				if !e.IsDir {
					return fmt.Errorf("路径冲突，已存在同名文件: %s", seg)
				}
				break
			}
		}
		if !found {
			newEntry, err := w.Mkdir(ctx, currentFid, encSeg)
			if err != nil {
				return fmt.Errorf("创建目录 %s 失败: %w", seg, err)
			}
			currentFid = newEntry.ID
		}
	}
	return nil
}
