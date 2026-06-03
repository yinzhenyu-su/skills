//go:build cgo && !nofuse

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/daemon"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
	"github.com/yinzhenyu/skills/qrypt/internal/rpc"
)

var _ rpc.RPCHost = (*daemon.Daemon)(nil) // compile-time check


func init() {
	var mountCmd = &cobra.Command{
		Use:   "mount [name]",
		Short: "Mount cloud drive to a local directory",
		Long: `Mount one or more cloud drives to local directories.

If no arguments and no flags are given, the default mount (or first enabled) is mounted.
Use --all to mount every enabled instance.
Use <name> to mount a specific instance by name.

This command starts the qrypt daemon automatically — no separate qryptd needed.
Use --daemon for headless mode (daemon without FUSE mount).`,
		Run: runMount,
	}
	mountCmd.Flags().StringP("config", "f", "", "配置文件路径 (默认搜索 qrypt.toml)")
	mountCmd.Flags().String("drive-type", "", "驱动类型: quark, yun139 (默认: 配置文件 backend.type)")
	mountCmd.Flags().StringP("cookie", "c", "", "Quark Drive Cookie")
	mountCmd.Flags().StringP("cache", "a", "", "本地缓存目录")
	mountCmd.Flags().StringP("mount", "m", "", "本地挂载点")
	mountCmd.Flags().StringP("password", "p", "", "Rclone 密码")
	mountCmd.Flags().StringP("salt", "s", "", "Rclone salt (可选)")
	mountCmd.Flags().StringP("root-path", "r", "", "网盘挂载路径")
	mountCmd.Flags().String("log-level", "", "日志级别: debug, info, warn, error")
	mountCmd.Flags().Bool("all", false, "挂载所有已启用的实例")
	mountCmd.Flags().Bool("daemon", false, "守护进程模式（不挂载 FUSE，仅启动服务端）")
	mountCmd.Flags().Bool("stop-daemon", false, "停止运行中的 daemon")

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
	if stop, _ := cmd.Flags().GetBool("stop-daemon"); stop {
		stopRunningDaemon()
		return
	}

	socketPath := rpc.FindSocketPath()

	// If daemon is already running, delegate via RPC
	if rpc.IsDaemonRunning(socketPath) {
		if daemonMode, _ := cmd.Flags().GetBool("daemon"); daemonMode {
			fmt.Println("daemon 已经在运行中")
			return
		}
		delegateMountToRunningDaemon(cmd, args, socketPath)
		return
	}

	// --- No daemon running — start one directly ---

	daemonMode, _ := cmd.Flags().GetBool("daemon")
	configPath, _ := cmd.Flags().GetString("config")
	logLevel, _ := cmd.Flags().GetString("log-level")

	// Load config (capture errors instead of os.Exit)
	cfgPath := configPath
	if cfgPath == "" {
		cfgPath = config.FindConfigFile()
	}

	var (
		cfg          *config.Config
		usedPath     string
		startupErrs  []string
	)

	cfg = config.DefaultConfig()
	if cfgPath != "" {
		usedPath = cfgPath
		if loadedCfg, validRes, loadErr := config.LoadConfig(usedPath); loadErr != nil {
			startupErrs = append(startupErrs, fmt.Sprintf("加载配置文件失败: %v", loadErr))
		} else if validRes != nil && !validRes.Valid {
			var msgs []string
			for _, c := range validRes.Checks {
				if c.Status == "error" {
					msgs = append(msgs, fmt.Sprintf("[%s] %s", c.Field, c.Message))
				}
			}
			startupErrs = append(startupErrs, "配置文件校验失败:\n"+strings.Join(msgs, "\n"))
		} else {
			cfg = loadedCfg
		}
	} else if daemonMode {
		startupErrs = append(startupErrs, "未找到配置文件。请使用 --config 指定或创建 qrypt.toml")
	}

	// Override log level from flag
	if logLevel != "" {
		cfg.Log.Level = logLevel
	}

	// Init logger (fallback on failure)
	rotateCfg := logging.DefaultRotateConfig
	if cfg.Log.MaxSize > 0 {
		rotateCfg.MaxSize = cfg.Log.MaxSize
	}
	if cfg.Log.MaxBackups > 0 {
		rotateCfg.MaxBackups = cfg.Log.MaxBackups
	}
	if cfg.Log.MaxAge > 0 {
		rotateCfg.MaxAge = cfg.Log.MaxAge
	}
	if cfg.Log.Compress != nil {
		rotateCfg.Compress = *cfg.Log.Compress
	}
	logger, err := logging.New(cfg.Log.Level, cfg.Log.File, &rotateCfg)
	if err != nil {
		startupErrs = append(startupErrs, fmt.Sprintf("日志初始化失败: %v", err))
	} else {
		logging.L = logger
		defer logger.Close()
	}

	logging.L.Infof("qrypt mount v%s starting...\n", version)

	// Create daemon with all components
	d := daemon.NewDaemonWithPath(cfg, usedPath, version)

	// Store startup errors in daemon so they can be served via WS
	if len(startupErrs) > 0 {
		d.SetStartupError(strings.Join(startupErrs, "; "))
	}

	// Start WS server early, before any fatal error, so client can query startup status
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := rpc.NewWSServer(d, socketPath)
	if daemonMode {
		srv.SetHeadless(true)
	}
	if err := srv.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 启动服务器失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("daemon 正在监听 %s\n", socketPath)

	// If startup failed, keep daemon alive for client to query error via RPC
	if len(startupErrs) > 0 {
		for _, e := range startupErrs {
			fmt.Fprintln(os.Stderr, e)
		}
		srv.SetHeadless(true)
	} else if !daemonMode {
		targets := resolveMountTargets(cmd, args, cfg)
		for _, m := range targets {
			if err := d.Start(ctx, m.Name); err != nil {
				logging.L.Errorf("启动挂载 %s 失败: %v\n", m.Name, err)
				fmt.Fprintf(os.Stderr, "  %s: 启动失败: %v\n", m.Name, err)
			} else {
				fmt.Printf("  %s: 已挂载\n", m.Name)
			}
		}
	} else {
		fmt.Println("守护进程模式（无 FUSE 挂载）")
		fmt.Println("按 Ctrl+C 停止")
	}

	// Signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigChan:
		fmt.Println("\n正在停止...")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		d.DaemonShutdown(shutdownCtx)
		srv.Stop()
	case <-srv.Done():
		// shutdown was initiated via RPC; already handled
	}
	logging.L.Infof("daemon stopped\n")
}

func stopRunningDaemon() {
	socketPath := rpc.FindSocketPath()
	if !rpc.IsDaemonRunning(socketPath) {
		fmt.Println("daemon 未在运行")
		return
	}
	client, err := rpc.DialWS(socketPath)
	if err != nil {
		fmt.Printf("无法连接到 daemon: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()
	_, rpcErr := client.Call("shutdown", nil)
	if rpcErr != nil {
		fmt.Printf("停止 daemon 失败: %v\n", rpcErr)
		os.Exit(1)
	}
	fmt.Println("daemon 已停止")
}

func delegateMountToRunningDaemon(cmd *cobra.Command, args []string, socketPath string) {
	client, err := rpc.DialWS(socketPath)
	if err != nil {
		fmt.Printf("无法连接到 daemon: %v\n", err)
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
		fmt.Println("已启动所有已启用挂载")
		return
	}

	configPath, _ := cmd.Flags().GetString("config")
	_, cfg, _, _ := config.LoadConfigAuto(configPath)
	if cfg == nil {
		resp, rpcErr := client.Call("start", map[string]string{"name": ""})
		if rpcErr != nil {
			fmt.Printf("RPC 错误: %v\n", rpcErr)
			os.Exit(1)
		}
		if resp.Error != nil {
			fmt.Printf("启动挂载失败: %s\n", resp.Error.Message)
			os.Exit(1)
		}
		fmt.Println("已启动默认挂载")
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
		fmt.Printf("  %s: 已挂载\n", m.Name)
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
