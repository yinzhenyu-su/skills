package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
	"github.com/yinzhenyu/skills/qrypt/internal/sync"
)

func runPull(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)

	remotePath := args[0]
	mountName := resolveMount(cmd, &remotePath)
	drv := loadToolDriverForMount(cfg, cipher, mountName)

	localPath := ""
	if len(args) >= 2 {
		localPath = args[1]
	}

	update, _ := cmd.Flags().GetBool("update")
	transfers, _ := cmd.Flags().GetInt("transfers")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	fullRemotePath := resolveFullPath(cfg.RootPath(), remotePath)
	parentPath := filepath.Dir(fullRemotePath)
	baseName := filepath.Base(fullRemotePath)

	resolver, ok := drv.(pathResolver)
	if !ok {
		fmt.Printf("该驱动不支持路径解析\n")
		os.Exit(1)
	}

	// Check if target is a directory
	targetFid, err := resolver.ResolvePath(context.Background(), fullRemotePath)
	if err == nil {
		_, errList := drv.List(context.Background(), targetFid)
		if errList == nil {
			// It's a directory
			if localPath == "" {
				localPath = baseName
			}
			
			if !dryRun {
				os.MkdirAll(localPath, 0755)
			}
			
			pool := sync.NewWorkerPool(drv, cipher, transfers, dryRun, update)
			pool.Start(context.Background())

			fmt.Printf("开始递归下载目录: %s\n", remotePath)
			scanErr := sync.ScanRemoteForDownload(context.Background(), targetFid, localPath, drv, cipher, pool)
			pool.Wait()
			if scanErr != nil {
				fmt.Printf("扫描远端目录失败: %v\n", scanErr)
				return
			}
			fmt.Printf("批量任务处理完毕\n")
			return
		}
	}

	// Target is a file
	parentFid, err := resolver.ResolvePath(context.Background(), parentPath)
	if err != nil {
		fmt.Printf("无法解析路径: %v\n", err)
		os.Exit(1)
	}

	entries, err := drv.List(context.Background(), parentFid)
	if err != nil {
		fmt.Printf("无法列出目录内容: %v\n", err)
		os.Exit(1)
	}

	encName := cipher.EncryptSegment(baseName)
	var targetEntry drive.Entry
	found := false
	for _, e := range entries {
		if e.Name == encName {
			targetEntry = e
			found = true
			break
		}
	}
	if !found {
		fmt.Printf("文件未找到: %s\n", baseName)
		os.Exit(1)
	}

	if localPath == "" {
		localPath = baseName
	}

	pool := sync.NewWorkerPool(drv, cipher, transfers, dryRun, update)
	pool.Start(context.Background())

	pool.Submit(sync.TransferJob{
		Type:        sync.JobTypeDownload,
		LocalPath:   localPath,
		RemoteName:  baseName,
		RemoteEntry: targetEntry,
		Size:        targetEntry.Size,
	})

	pool.Wait()
	fmt.Printf("\n完成下载\n")
}
