package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/rpc"
)

var osExit = os.Exit

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
	flagMount, _ := cmd.Flags().GetString("mount")

	var pathMount string
	var cleanPath string
	if path != nil {
		pathMount, cleanPath = ParseMountPath(*path)
	}

	if flagMount != "" && pathMount != "" {
		if flagMount != pathMount {
			cmd.PrintErrln(fmt.Sprintf("错误: --mount 标志与路径前缀指定了不同的挂载实例 ('%s' vs '%s')", flagMount, pathMount))
			osExit(1)
		}
		*path = cleanPath
		return flagMount
	}

	if flagMount != "" {
		return flagMount
	}

	if pathMount != "" {
		*path = cleanPath
		return pathMount
	}

	return ""
}

// ensureDaemon checks if a daemon is running and auto-starts one if not.
// Returns a connected WSClient. Caller MUST close the client when done.
func ensureDaemon() (*rpc.WSClient, error) {
	socketPath := rpc.FindSocketPath()
	if rpc.IsDaemonRunning(socketPath) {
		return rpc.DialWS(socketPath)
	}
	if err := startDaemonHeadless(); err != nil {
		return nil, err
	}
	for i := 0; i < 50; i++ {
		if rpc.IsDaemonRunning(socketPath) {
			return rpc.DialWS(socketPath)
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

// resolveMountConfig returns the mount name to use and its resolved config.
// If mountName is empty, returns the default mount.
// Returns error if the named mount is not found.
func resolveMountConfig(cfg *config.Config, mountName string) (*config.ResolvedMountConfig, error) {
	var mountCfg *config.MountInstance
	if mountName == "" {
		mountCfg = config.FindDefaultMount(cfg)
	} else {
		for _, m := range cfg.Mounts {
			if m.Name == mountName {
				mountCfg = &m
				break
			}
		}
	}
	if mountCfg == nil {
		if mountName != "" {
			return nil, fmt.Errorf("未找到挂载实例: %s", mountName)
		}
		return nil, fmt.Errorf("配置中未找到启用的挂载实例")
	}
	return cfg.MergeInstanceConfig(*mountCfg), nil
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

func StripRootPath(rootPath, displayPath string) string {
	if rootPath == "/" || rootPath == "" {
		return displayPath
	}

	if displayPath == rootPath {
		return "/"
	}

	if strings.HasPrefix(displayPath, rootPath+"/") {
		return displayPath[len(rootPath):]
	}

	return displayPath
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
