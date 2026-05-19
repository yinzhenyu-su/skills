package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/daemon"
	"github.com/yinzhenyu/skills/qrypt/internal/log"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "", "配置文件的路径（默认自动搜索）")
	logLevel := flag.String("log-level", "info", "日志级别: debug, info, warn, error")
	socketPath := flag.String("socket", config.WorkDir()+"/qryptd.sock", "Unix socket 路径")
	showVersion := flag.Bool("version", false, "显示版本信息")
	flag.Parse()

	if *showVersion {
		fmt.Printf("qryptd version %s\n", version)
		os.Exit(0)
	}

	// Find and load config
	cfgPath := *configPath
	if cfgPath == "" {
		cfgPath = config.FindConfigFile()
	}
	if cfgPath == "" {
		fmt.Fprintf(os.Stderr, "错误: 未找到配置文件。请使用 --config 指定或创建 qrypt.toml\n")
		os.Exit(1)
	}

	cfg, _, err := config.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 加载配置文件失败: %v\n", err)
		os.Exit(1)
	}

	// Override log level if specified
	if *logLevel != "" {
		cfg.Log.Level = *logLevel
	}

	// Initialize logger
	rotateCfg := log.DefaultRotateConfig
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
	logger, err := log.New(cfg.Log.Level, cfg.Log.File, &rotateCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "日志初始化失败: %v\n", err)
		os.Exit(1)
	}
	log.L = logger
	defer logger.Close()

	log.L.Infof("qryptd v%s starting...\n", version)

	// Create daemon
	d := daemon.NewDaemon(cfg, version)

	// Create server
	*socketPath = config.ExpandHome(*socketPath)
	srv := daemon.NewServer(d, *socketPath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	if err := srv.Start(ctx); err != nil {
		log.L.Errorf("启动服务器失败: %v\n", err)
		fmt.Fprintf(os.Stderr, "错误: 启动服务器失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("qryptd 正在监听 %s\n", *socketPath)
	fmt.Println("按 Ctrl+C 停止")

	// Signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Auto-start mount
	if err := d.Start(ctx); err != nil {
		log.L.Errorf("自动挂载失败: %v\n", err)
		fmt.Fprintf(os.Stderr, "自动挂载失败: %v\n", err)
		// Don't exit - daemon can still serve config requests
	}

	<-sigChan
	fmt.Println("\n正在停止...")

	// Graceful shutdown
	d.Stop(ctx)
	srv.Stop()
	log.L.Infof("qryptd stopped\n")
}
