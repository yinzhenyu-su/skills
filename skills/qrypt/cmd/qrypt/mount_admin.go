package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

func runMountList(cmd *cobra.Command, args []string) {
	client, err := ensureDaemon()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
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
	client, err := ensureDaemon()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
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
	fmt.Printf("挂载实例 %q 已启动\n", args[0])
}

func runMountStop(cmd *cobra.Command, args []string) {
	client, err := ensureDaemon()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
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
	fmt.Printf("挂载实例 %q 已停止\n", args[0])
}
