package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

func runStatus(cmd *cobra.Command, args []string) {
	configPath, _ := cmd.Flags().GetString("config")
	if configPath == "" {
		configPath = config.FindConfigFile()
	}
	cfg, _, _ := config.LoadConfig(configPath)

	fmt.Println("=== Qrypt Status ===")
	fmt.Printf("配置文件:    %s\n", configPath)
	if configPath != "" {
		fmt.Printf("  缓存目录:  %s\n", cfg.Cache.Dir)
		fmt.Printf("  缓存上限:  %s\n", cfg.Cache.MaxSize)
		fmt.Printf("  挂载点:    %s\n", cfg.Mount.Point)
	}

	// Cache metrics
	cacheDir := config.ExpandHome(cfg.Cache.Dir)
	printCacheMetrics(cacheDir)

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

func printCacheMetrics(cacheDir string) {
	fmt.Println()
	fmt.Printf("缓存目录:    %s\n", cacheDir)

	// pending journal
	journalPath := filepath.Join(cacheDir, "pending.jsonl")
	if fi, err := os.Stat(journalPath); err == nil {
		lines := countLines(journalPath)
		fmt.Printf("  journal:    %s, %d 条\n", formatBytes(fi.Size()), lines)
	} else {
		fmt.Printf("  journal:    无\n")
	}

	// staging dir
	stagingDir := filepath.Join(cacheDir, "staging")
	if entries, err := os.ReadDir(stagingDir); err == nil {
		var totalSize int64
		for _, e := range entries {
			if fi, err := e.Info(); err == nil {
				totalSize += fi.Size()
			}
		}
		fmt.Printf("  staging:    %d 个文件, 共 %s\n", len(entries), formatBytes(totalSize))
	} else {
		fmt.Printf("  staging:    无\n")
	}

	// reading cache
	readingDir := filepath.Join(cacheDir, "reading")
	if entries, err := filepath.Glob(filepath.Join(readingDir, "*.dec.batch")); err == nil {
		fmt.Printf("  读缓存:     %d 个分块\n", len(entries))
	} else {
		fmt.Printf("  读缓存:     无\n")
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
