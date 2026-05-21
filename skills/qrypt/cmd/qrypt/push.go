package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/daemon"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

func runPush(cmd *cobra.Command, args []string) {
	socketPath := daemon.FindSocketPath()
	if !daemon.IsDaemonRunning(socketPath) {
		fmt.Println("错误: qryptd 未运行，请先启动 qryptd")
		fmt.Println("提示: 运行 qryptd 启动守护进程，以使用推送功能")
		os.Exit(1)
	}
	runPushViaDaemon(cmd, args, socketPath)
}

func runPushViaDaemon(cmd *cobra.Command, args []string, socketPath string) {
	localPath := args[0]
	remotePath := ""
	if len(args) >= 2 {
		remotePath = args[1]
	}
	mountName := resolveMount(cmd, &remotePath)

	update, _ := cmd.Flags().GetBool("update")
	transfers, _ := cmd.Flags().GetInt("transfers")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	password, _ := cmd.Flags().GetString("password")
	salt, _ := cmd.Flags().GetString("salt")

	isStdin := localPath == "-"
	source := localPath
	plainSize := int64(-1)
	var tmpFile string

	if isStdin {
		f, err := os.CreateTemp("", "qrypt-stdin-*")
		if err != nil {
			fmt.Printf("创建临时文件失败: %v\n", err)
			os.Exit(1)
		}
		written, err := io.Copy(f, os.Stdin)
		if err != nil {
			f.Close()
			os.Remove(f.Name())
			fmt.Printf("读取标准输入失败: %v\n", err)
			os.Exit(1)
		}
		f.Close()
		tmpFile = f.Name()
		source = tmpFile
		plainSize = written
	} else if fi, err := os.Stat(localPath); err == nil && !fi.IsDir() {
		plainSize = fi.Size()
	}

	// Connect to daemon via WebSocket (single connection for RPC + events)
	client, err := daemon.DialWS(socketPath)
	if err != nil {
		fmt.Printf("无法连接到 qryptd: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	// Send push_start RPC
	resp, err := client.Call("push_start", protocol.PushStartParams{
		MountName: mountName,
		Source:    source,
		Remote:    remotePath,
		Password:  password,
		Salt:      salt,
		Transfers: transfers,
		Update:    update,
		DryRun:    dryRun,
		PlainSize: plainSize,
	})
	if err != nil {
		fmt.Printf("RPC 调用失败: %v\n", err)
		os.Exit(1)
	}
	if resp.Error != nil {
		fmt.Printf("启动推送失败: %s\n", resp.Error.Message)
		os.Exit(1)
	}

	// Parse task ID from result
	resultData, _ := json.Marshal(resp.Result)
	var startResult protocol.PushStartResult
	json.Unmarshal(resultData, &startResult)

	fmt.Printf("推送任务已启动: %s\n", startResult.TaskID)

	// Subscribe to events on the same connection
	evtCh, err := client.SubscribeEvents()
	if err != nil {
		fmt.Printf("订阅事件失败: %v\n", err)
		if tmpFile != "" {
			os.Remove(tmpFile)
		}
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
			fmt.Printf("推送开始\n")
		case "uploading":
			fmt.Printf("上传: %s  %d/%d\n", progress.File, progress.Bytes, progress.Total)
		case "completed":
			fmt.Printf("推送完成\n")
			if tmpFile != "" {
				os.Remove(tmpFile)
			}
			return
		case "failed":
			fmt.Printf("推送失败: %s\n", progress.Error)
			if tmpFile != "" {
				os.Remove(tmpFile)
			}
			os.Exit(1)
		}
	}
}


