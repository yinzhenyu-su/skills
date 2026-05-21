package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/daemon"
)

func runStatus(cmd *cobra.Command, args []string) {
	configPath, _ := cmd.Flags().GetString("config")
	loadedPath, cfg, vr, _ := config.LoadConfigAuto(configPath)

	fmt.Println("=== Qrypt Status ===")
	fmt.Printf("配置文件:    %s\n", loadedPath)
	fmt.Printf("版本:        %s\n", cfg.Version)

	if vr != nil && !vr.Valid {
		fmt.Printf("配置状态:    无效 (运行 qrypt validate 查看详情)\n")
	} else {
		fmt.Printf("配置状态:    有效\n")
	}

	if len(cfg.Mounts) > 0 {
		fmt.Println()
		fmt.Println("挂载实例:")
		for _, m := range cfg.Mounts {
			rc := cfg.MergeInstanceConfig(m)
			state := "已配置"
			if rc.Enabled {
				state = "已启用"
			}
			fmt.Printf("  %s: %s (%s, %s)\n", m.Name, rc.MountPoint, m.Type, state)
		}
	}

	fmt.Println()
	for _, m := range cfg.Mounts {
		cacheDir := filepath.Join(config.WorkDir(), "cache", m.Name)
		fmt.Printf("缓存目录(%s): %s\n", m.Name, cacheDir)
		printCacheMetrics(cacheDir)
	}

	if daemon.IsDaemonRunning(daemon.FindSocketPath()) {
		fmt.Println()
		fmt.Println("运行状态:    daemon 运行中")
	} else {
		fmt.Println()
		fmt.Println("运行状态:    daemon 未运行 (运行 'qrypt mount' 启动)")
	}

	if len(cfg.Mounts) > 0 {
		fmt.Println()
		fmt.Println("挂载状态:")
		for _, m := range cfg.Mounts {
			mountPoint := config.ExpandHome(m.MountPoint)
			status := "未挂载"
			if isMounted(mountPoint) {
				status = "已挂载"
			}
			fmt.Printf("  %s: %s (%s)\n", m.Name, status, mountPoint)
		}
	}
}

func printCacheMetrics(cacheDir string) {
	fmt.Printf("  缓存目录:  %s\n", cacheDir)

	journalPath := filepath.Join(cacheDir, "pending.jsonl")
	if fi, err := os.Stat(journalPath); err == nil {
		lines := countLines(journalPath)
		fmt.Printf("    journal:  %s, %d 条\n", formatBytes(fi.Size()), lines)
	} else {
		fmt.Printf("    journal:  无\n")
	}

	stagingDir := filepath.Join(cacheDir, "staging")
	if entries, err := os.ReadDir(stagingDir); err == nil {
		var totalSize int64
		for _, e := range entries {
			if fi, err := e.Info(); err == nil {
				totalSize += fi.Size()
			}
		}
		fmt.Printf("    staging:  %d 个文件, 共 %s\n", len(entries), formatBytes(totalSize))
	} else {
		fmt.Printf("    staging:  无\n")
	}
}

func countLines(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	if len(data) == 0 {
		return 0
	}
	n := 1
	for _, b := range data {
		if b == '\n' {
			n++
		}
	}
	return n
}

func isMounted(point string) bool {
	cmd := exec.Command("mount")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), point) && strings.Contains(string(out), "fuse")
}
