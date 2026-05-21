package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/daemon"
)

// ParseMountPath parses "mount_name:path" format.
// Returns (mountName, path). If no prefix, mountName is empty.
func ParseMountPath(s string) (mountName, path string) {
	if s == "" || s[0] == '.' || s[0] == '/' || s[0] == '~' {
		return "", s
	}
	idx := strings.Index(s, ":/")
	if idx <= 0 || idx > 32 {
		return "", s
	}
	candidate := s[:idx]
	if !config.ValidMountName(candidate) {
		return "", s
	}
	return candidate, s[idx+1:]
}

// resolveMount returns the mount name from --flag or path prefix.
func resolveMount(cmd *cobra.Command, path *string) string {
	if mountName, _ := cmd.Flags().GetString("mount"); mountName != "" {
		return mountName
	}
	if path != nil {
		mountName, cleanPath := ParseMountPath(*path)
		if mountName != "" {
			*path = cleanPath
			return mountName
		}
	}
	return ""
}

// ensureDaemon checks if a daemon is running and auto-starts one if not.
// Returns a connected WSClient. Caller MUST close the client when done.
func ensureDaemon() (*daemon.WSClient, error) {
	socketPath := daemon.FindSocketPath()
	if daemon.IsDaemonRunning(socketPath) {
		return daemon.DialWS(socketPath)
	}
	if err := startDaemonHeadless(); err != nil {
		return nil, err
	}
	for i := 0; i < 50; i++ {
		if daemon.IsDaemonRunning(socketPath) {
			return daemon.DialWS(socketPath)
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil, fmt.Errorf("daemon 未能启动")
}

// startDaemonHeadless starts a headless daemon process in the background.
func startDaemonHeadless() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("无法获取可执行文件路径: %w", err)
	}
	cmd := exec.Command(exe, "mount", "--daemon")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 daemon 失败: %w", err)
	}
	go cmd.Wait()
	return nil
}

func maskStr(s string) string {
	if s == "" {
		return "(未设置)"
	}
	if len(s) <= 4 {
		return "****"
	}
	return s[:1] + "****" + s[len(s)-1:]
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n/div >= unit && exp < len("KMGTPE")-1 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
