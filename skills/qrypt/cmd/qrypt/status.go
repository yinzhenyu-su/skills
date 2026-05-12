package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

func runStatus(cmd *cobra.Command, args []string) {
	configPath, _ := cmd.Flags().GetString("config")
	if configPath == "" {
		configPath = config.FindConfigFile()
	}
	cfg, _ := config.LoadConfig(configPath)

	fmt.Println("=== Qrypt Status ===")
	fmt.Printf("配置文件:    %s\n", configPath)
	if configPath != "" {
		fmt.Printf("  缓存目录:  %s\n", cfg.Cache.Dir)
		fmt.Printf("  缓存上限:  %s\n", cfg.Cache.MaxSize)
		fmt.Printf("  挂载点:    %s\n", cfg.Mount.Point)
	}

	procRunning := checkQryptProcess()
	fmt.Println()
	if procRunning {
		fmt.Println("运行状态:    运行中")
	} else {
		fmt.Println("运行状态:    未运行")
	}

	fmt.Println()
	if cfg.Mount.Point != "" {
		mountPoint := config.ExpandHome(cfg.Mount.Point)
		if isMounted(mountPoint) {
			fmt.Printf("挂载状态:    已挂载到 %s\n", mountPoint)
		} else {
			fmt.Printf("挂载状态:    未挂载\n")
		}
	}
}

func checkQryptProcess() bool {
	cmd := exec.Command("pgrep", "-f", "qrypt mount")
	out, err := cmd.Output()
	return err == nil && len(out) > 0
}

func isMounted(point string) bool {
	cmd := exec.Command("mount")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), point) && strings.Contains(string(out), "fuse")
}
