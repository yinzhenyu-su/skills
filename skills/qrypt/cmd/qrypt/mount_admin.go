package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/daemon"
)

func runMountList(cmd *cobra.Command, args []string) {
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

	mm := daemon.NewMountManager(cfg)
	if err := mm.Start(context.Background(), name); err != nil {
		fmt.Printf("启动挂载实例 %q 失败: %v\n", name, err)
		os.Exit(1)
	}
	fmt.Printf("挂载实例 %q 已启动\n", name)
}

func runMountStop(cmd *cobra.Command, args []string) {
	cfg := loadToolCfgOnly(cmd)
	name := args[0]

	mm := daemon.NewMountManager(cfg)
	if err := mm.Stop(context.Background(), name); err != nil {
		fmt.Printf("停止挂载实例 %q 失败: %v\n", name, err)
		os.Exit(1)
	}
	fmt.Printf("挂载实例 %q 已停止\n", name)
}
