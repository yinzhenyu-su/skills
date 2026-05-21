package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

func runPull(cmd *cobra.Command, args []string) {
	client, err := ensureDaemon()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

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

	evtCh, err := client.SubscribeEvents()
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
