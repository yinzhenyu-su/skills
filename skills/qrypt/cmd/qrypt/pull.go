package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/daemon"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
	"github.com/yinzhenyu/skills/qrypt/internal/sync"
)

func runPull(cmd *cobra.Command, args []string) {
	socketPath := daemon.FindSocketPath()
	if daemon.IsDaemonRunning(socketPath) {
		runPullViaDaemon(cmd, args, socketPath)
	} else {
		runPullDirect(cmd, args)
	}
}

func runPullViaDaemon(cmd *cobra.Command, args []string, socketPath string) {
	remotePath := args[0]
	mountName := resolveMount(cmd, &remotePath)
	localPath := ""
	if len(args) >= 2 {
		localPath = args[1]
	}

	update, _ := cmd.Flags().GetBool("update")
	transfers, _ := cmd.Flags().GetInt("transfers")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	password, _ := cmd.Flags().GetString("password")
	salt, _ := cmd.Flags().GetString("salt")

	// Connect to daemon
	client, err := daemon.DialClient(socketPath)
	if err != nil {
		fmt.Printf("无法连接到 qryptd: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	resp, rpcErr := client.Call("pull_start", protocol.PullStartParams{
		MountName: mountName,
		Remote:    remotePath,
		Local:     localPath,
		Password:  password,
		Salt:      salt,
		Transfers: transfers,
		Update:    update,
		DryRun:    dryRun,
	})
	if rpcErr != nil {
		fmt.Printf("RPC 错误: %v\n", rpcErr)
		os.Exit(1)
	}
	if resp.Error != nil {
		fmt.Printf("启动下载失败: %s\n", resp.Error.Message)
		os.Exit(1)
	}

	resultData, _ := json.Marshal(resp.Result)
	var startResult protocol.PushStartResult
	json.Unmarshal(resultData, &startResult)

	fmt.Printf("下载任务已启动: %s\n", startResult.TaskID)

	// Subscribe to events on second connection
	client2, err := daemon.DialClient(socketPath)
	if err != nil {
		return
	}
	defer client2.Close()

	evtCh, err := client2.SubscribeEvents()
	if err != nil {
		fmt.Printf("订阅事件失败: %v\n", err)
		os.Exit(1)
	}

	for evt := range evtCh {
		data, _ := json.Marshal(evt.Data)
		var progress protocol.PushProgressData
		if err := json.Unmarshal(data, &progress); err != nil {
			continue
		}
		if progress.TaskID != startResult.TaskID {
			continue
		}

		switch progress.State {
		case "started":
			fmt.Printf("下载开始\n")
		case "downloading":
			fmt.Printf("下载: %s  %d/%d\n", progress.File, progress.Bytes, progress.Total)
		case "completed":
			fmt.Printf("下载完成\n")
			return
		case "failed":
			fmt.Printf("下载失败: %s\n", progress.Error)
			os.Exit(1)
		}
	}
}

func runPullDirect(cmd *cobra.Command, args []string) {
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

	fullRemotePath := config.ResolveFullPath(cfg.RootPath(), remotePath)
	parentPath := filepath.Dir(fullRemotePath)
	baseName := filepath.Base(fullRemotePath)

	resolver, ok := drv.(drive.PathResolver)
	if !ok {
		fmt.Printf("该驱动不支持路径解析\n")
		os.Exit(1)
	}

	targetFid, err := resolver.ResolvePath(context.Background(), fullRemotePath)
	if err == nil {
		_, errList := drv.List(context.Background(), targetFid)
		if errList == nil {
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
