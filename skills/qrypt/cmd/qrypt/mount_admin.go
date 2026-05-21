package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/daemon"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

func runMountList(cmd *cobra.Command, args []string) {
	socketPath := daemon.FindSocketPath()
	if !daemon.IsDaemonRunning(socketPath) {
		fmt.Println("错误: qryptd 未运行，请先启动 qryptd")
		fmt.Println("提示: 运行 qryptd 启动守护进程，以查看挂载列表")
		os.Exit(1)
	}

	client, err := daemon.DialWS(socketPath)
	if err != nil {
		fmt.Printf("无法连接到 qryptd: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	resp, rpcErr := client.Call("mount_list", nil)
	if rpcErr != nil {
		fmt.Printf("RPC 错误: %v\n", rpcErr)
		os.Exit(1)
	}
	if resp.Error != nil {
		fmt.Printf("获取挂载列表失败: %s\n", resp.Error.Message)
		os.Exit(1)
	}

	data, _ := json.Marshal(resp.Result)
	var summaries []protocol.MountSummary
	json.Unmarshal(data, &summaries)

	fmt.Printf("%-20s %-12s %-30s %-10s\n", "NAME", "STATE", "MOUNT POINT", "TYPE")
	fmt.Println("--------------------------------------------------------------------------")
	for _, s := range summaries {
		fmt.Printf("%-20s %-12s %-30s %-10s\n", s.Name, s.State, s.MountPoint, s.DriveType)
	}
}

func runMountStart(cmd *cobra.Command, args []string) {
	socketPath := daemon.FindSocketPath()
	if !daemon.IsDaemonRunning(socketPath) {
		fmt.Println("错误: qryptd 未运行，请先启动 qryptd")
		fmt.Println("提示: 运行 qryptd 启动守护进程，以管理挂载")
		os.Exit(1)
	}

	client, err := daemon.DialWS(socketPath)
	if err != nil {
		fmt.Printf("无法连接到 qryptd: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	resp, rpcErr := client.Call("start", map[string]string{"name": args[0]})
	if rpcErr != nil {
		fmt.Printf("RPC 错误: %v\n", rpcErr)
		os.Exit(1)
	}
	if resp.Error != nil {
		fmt.Printf("启动失败: %s\n", resp.Error.Message)
		os.Exit(1)
	}
	fmt.Printf("挂载实例 %q 已通过 qryptd 启动\n", args[0])
}

func runMountStop(cmd *cobra.Command, args []string) {
	socketPath := daemon.FindSocketPath()
	if !daemon.IsDaemonRunning(socketPath) {
		fmt.Println("错误: qryptd 未运行，请先启动 qryptd")
		fmt.Println("提示: 运行 qryptd 启动守护进程，以管理挂载")
		os.Exit(1)
	}

	client, err := daemon.DialWS(socketPath)
	if err != nil {
		fmt.Printf("无法连接到 qryptd: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	resp, rpcErr := client.Call("stop", map[string]string{"name": args[0]})
	if rpcErr != nil {
		fmt.Printf("RPC 错误: %v\n", rpcErr)
		os.Exit(1)
	}
	if resp.Error != nil {
		fmt.Printf("停止失败: %s\n", resp.Error.Message)
		os.Exit(1)
	}
	fmt.Printf("挂载实例 %q 已通过 qryptd 停止\n", args[0])
}
