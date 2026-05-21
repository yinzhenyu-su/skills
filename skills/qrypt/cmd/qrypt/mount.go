//go:build cgo && !nofuse

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/daemon"
)

func init() {
	var mountCmd = &cobra.Command{
		Use:   "mount [name]",
		Short: "Mount cloud drive to a local directory",
		Long: `Mount one or more cloud drives to local directories.

If no arguments and no flags are given, the default mount (or first enabled) is mounted.
Use --all to mount every enabled instance.
Use <name> to mount a specific instance by name.

FUSE flags (--cookie, --password, etc.) can only be used when mounting a single instance.`,
		Run: runMount,
	}
	mountCmd.Flags().StringP("config", "f", "", "配置文件路径 (默认搜索 qrypt.toml)")
	mountCmd.Flags().String("drive-type", "", "驱动类型: quark, yun139 (默认: 配置文件 drive.type)")
	mountCmd.Flags().StringP("cookie", "c", "", "Quark Drive Cookie")
	mountCmd.Flags().StringP("cache", "a", "", "本地缓存目录")
	mountCmd.Flags().StringP("mount", "m", "", "本地挂载点")
	mountCmd.Flags().StringP("password", "p", "", "Rclone 密码")
	mountCmd.Flags().StringP("salt", "s", "", "Rclone salt (可选)")
	mountCmd.Flags().StringP("root-path", "r", "", "网盘挂载路径")
	mountCmd.Flags().String("log-level", "", "日志级别: debug, info, warn, error")
	mountCmd.Flags().Bool("all", false, "挂载所有已启用的实例")

	// Management subcommands
	mountCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "列出所有配置的挂载实例",
		Run:   runMountList,
	})
	mountCmd.AddCommand(&cobra.Command{
		Use:   "start <name>",
		Short: "启动指定挂载实例",
		Args:  cobra.ExactArgs(1),
		Run:   runMountStart,
	})
	mountCmd.AddCommand(&cobra.Command{
		Use:   "stop <name>",
		Short: "停止指定挂载实例",
		Args:  cobra.ExactArgs(1),
		Run:   runMountStop,
	})

	rootCmd.AddCommand(mountCmd)
}

func runMount(cmd *cobra.Command, args []string) {
	socketPath := daemon.FindSocketPath()
	if !daemon.IsDaemonRunning(socketPath) {
		fmt.Println("错误: qryptd 未运行，请先启动 qryptd")
		fmt.Println("提示: 运行 qryptd 启动守护进程，以使用挂载功能")
		os.Exit(1)
	}
	runMountViaDaemon(cmd, args, socketPath)
}

func runMountViaDaemon(cmd *cobra.Command, args []string, socketPath string) {
	client, err := daemon.DialWS(socketPath)
	if err != nil {
		fmt.Printf("无法连接到 qryptd: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	mountAll, _ := cmd.Flags().GetBool("all")
	if mountAll {
		resp, rpcErr := client.Call("start", nil)
		if rpcErr != nil {
			fmt.Printf("RPC 错误: %v\n", rpcErr)
			os.Exit(1)
		}
		if resp.Error != nil {
			fmt.Printf("启动挂载失败: %s\n", resp.Error.Message)
			os.Exit(1)
		}
		fmt.Println("已通过 qryptd 启动所有已启用挂载")
		return
	}

	// Resolve the mount name client-side
	configPath, _ := cmd.Flags().GetString("config")
	_, cfg, _, _ := config.LoadConfigAuto(configPath)
	if cfg == nil {
		// If config can't be loaded, ask daemon to start default
		resp, rpcErr := client.Call("start", map[string]string{"name": ""})
		if rpcErr != nil {
			fmt.Printf("RPC 错误: %v\n", rpcErr)
			os.Exit(1)
		}
		if resp.Error != nil {
			fmt.Printf("启动挂载失败: %s\n", resp.Error.Message)
			os.Exit(1)
		}
		fmt.Println("已通过 qryptd 启动默认挂载")
		return
	}

	targets := resolveMountTargets(cmd, args, cfg)
	if len(targets) == 0 {
		fmt.Println("没有匹配的挂载实例")
		os.Exit(1)
	}

	for _, m := range targets {
		resp, rpcErr := client.Call("start", map[string]string{"name": m.Name})
		if rpcErr != nil {
			fmt.Printf("  %s: RPC 错误: %v\n", m.Name, rpcErr)
			continue
		}
		if resp.Error != nil {
			fmt.Printf("  %s: 启动失败: %s\n", m.Name, resp.Error.Message)
			continue
		}
		fmt.Printf("  %s: 已通过 qryptd 挂载\n", m.Name)
	}
}

func resolveMountTargets(cmd *cobra.Command, args []string, cfg *config.Config) []*config.MountInstance {
	mountAll, _ := cmd.Flags().GetBool("all")

	switch {
	case mountAll:
		var targets []*config.MountInstance
		for i := range cfg.Mounts {
			rc := cfg.MergeInstanceConfig(cfg.Mounts[i])
			if rc.Enabled {
				targets = append(targets, &cfg.Mounts[i])
			}
		}
		return targets

	case len(args) > 0:
		name := args[0]
		for i := range cfg.Mounts {
			if cfg.Mounts[i].Name == name {
				return []*config.MountInstance{&cfg.Mounts[i]}
			}
		}
		fmt.Printf("挂载实例 %q 未找到\n", name)
		os.Exit(1)
		return nil

	default:
		m := config.FindDefaultMount(cfg)
		if m == nil {
			fmt.Println("没有可挂载的实例 — 请先配置 [[mounts]]")
			os.Exit(1)
		}
		return []*config.MountInstance{m}
	}
}


