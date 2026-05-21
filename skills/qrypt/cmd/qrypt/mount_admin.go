package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/daemon"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

func runMountList(cmd *cobra.Command, args []string) {
	socketPath := daemon.FindSocketPath()
	if daemon.IsDaemonRunning(socketPath) {
		client, err := daemon.DialWS(socketPath)
		if err == nil {
			defer client.Close()
			resp, rpcErr := client.Call("mount_list", nil)
			if rpcErr == nil && resp.Error == nil {
				data, _ := json.Marshal(resp.Result)
				var summaries []protocol.MountSummary
				json.Unmarshal(data, &summaries)
				fmt.Printf("%-20s %-12s %-30s %-10s\n", "NAME", "STATE", "MOUNT POINT", "TYPE")
				fmt.Println("--------------------------------------------------------------------------")
				for _, s := range summaries {
					fmt.Printf("%-20s %-12s %-30s %-10s\n", s.Name, s.State, s.MountPoint, s.DriveType)
				}
				return
			}
		}
		// Fall through to config-only display if RPC fails
	}

	cfg := loadToolCfgOnly(cmd)
	if len(cfg.Mounts) == 0 {
		fmt.Println("没有配置任何挂载实例")
		return
	}
	defaultMount := config.FindDefaultMount(cfg)
	fmt.Printf("%-20s %-12s %-30s %-10s  %s\n", "NAME", "STATE", "MOUNT POINT", "TYPE", "DEFAULT")
	fmt.Println("--------------------------------------------------------------------------")
	for _, m := range cfg.Mounts {
		rc := cfg.MergeInstanceConfig(m)
		state := "configured"
		if rc.Enabled {
			state = "enabled"
		}
		def := ""
		if defaultMount != nil && m.Name == defaultMount.Name {
			def = "default"
		}
		fmt.Printf("%-20s %-12s %-30s %-10s  %s\n", m.Name, state, rc.MountPoint, m.Type, def)
	}
}

func runMountStart(cmd *cobra.Command, args []string) {
	socketPath := daemon.FindSocketPath()
	if daemon.IsDaemonRunning(socketPath) {
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
		return
	}

	// Fallback: standalone
	cfg := loadToolCfgOnly(cmd)
	name := args[0]
	found := false
	for _, m := range cfg.Mounts {
		if m.Name == name {
			found = true
			break
		}
	}
	if !found {
		fmt.Printf("挂载实例 %q 未找到\n", name)
		os.Exit(1)
	}
	mm := daemon.NewMountManagerStandalone(cfg)
	if err := mm.Start(context.Background(), name); err != nil {
		fmt.Printf("启动挂载实例 %q 失败: %v\n", name, err)
		os.Exit(1)
	}
	fmt.Printf("挂载实例 %q 已启动\n", name)
}

func runMountStop(cmd *cobra.Command, args []string) {
	socketPath := daemon.FindSocketPath()
	if daemon.IsDaemonRunning(socketPath) {
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
		return
	}

	// Fallback: standalone
	cfg := loadToolCfgOnly(cmd)
	name := args[0]
	mm := daemon.NewMountManagerStandalone(cfg)
	if err := mm.Stop(context.Background(), name); err != nil {
		fmt.Printf("停止挂载实例 %q 失败: %v\n", name, err)
		os.Exit(1)
	}
	fmt.Printf("挂载实例 %q 已停止\n", name)
}
