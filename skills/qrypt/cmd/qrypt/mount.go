//go:build cgo && !nofuse

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	factory "github.com/yinzhenyu/skills/qrypt/internal/drive/factory"
	"github.com/yinzhenyu/skills/qrypt/internal/fs"
	"github.com/yinzhenyu/skills/qrypt/internal/log"
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

type mountTarget struct {
	instance config.MountInstance
	host     *fuse.FileSystemHost
	vfs      *fs.QryptFS
}

func runMount(cmd *cobra.Command, args []string) {
	configPath, _ := cmd.Flags().GetString("config")
	_, cfg, vr, err := config.LoadConfigAuto(configPath)
	if err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
		os.Exit(1)
	}

	if !vr.Valid {
		fmt.Println("配置文件校验失败:")
		for _, c := range vr.Checks {
			if c.Status == "error" {
				fmt.Printf("  [%s] %s\n", c.Field, c.Message)
			}
		}
		fmt.Println()
		fmt.Println("请修复配置文件后重试，或运行 qrypt validate 查看详细信息")
		os.Exit(1)
	}

	targets := resolveMountTargets(cmd, args, cfg)
	if len(targets) == 0 {
		fmt.Println("没有匹配的挂载实例")
		os.Exit(1)
	}

	if logLevel, _ := cmd.Flags().GetString("log-level"); logLevel != "" {
		cfg.Log.Level = logLevel
	}

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
		fmt.Printf("日志初始化失败: %v\n", err)
		os.Exit(1)
	}
	log.L = logger
	defer logger.Close()

	mountAllAndWait(targets, cfg)
}

func resolveMountTargets(cmd *cobra.Command, args []string, cfg *config.Config) []config.MountInstance {
	mountAll, _ := cmd.Flags().GetBool("all")
	hasFlags := hasMountOverrideFlags(cmd)

	switch {
	case mountAll:
		if hasFlags {
			fmt.Println("警告: --all 模式下忽略 FUSE 覆盖标志")
		}
		var targets []config.MountInstance
		for _, m := range cfg.Mounts {
			rc := cfg.MergeInstanceConfig(m)
			if rc.Enabled {
				targets = append(targets, m)
			}
		}
		return targets

	case len(args) > 0:
		name := args[0]
		for _, m := range cfg.Mounts {
			if m.Name == name {
				applyMountOverrides(cmd, &m)
				return []config.MountInstance{m}
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
		applyMountOverrides(cmd, m)
		return []config.MountInstance{*m}
	}
}

func hasMountOverrideFlags(cmd *cobra.Command) bool {
	for _, name := range []string{"cookie", "password", "salt", "mount", "root-path", "drive-type", "cache"} {
		if v, _ := cmd.Flags().GetString(name); v != "" {
			return true
		}
	}
	return false
}

func applyMountOverrides(cmd *cobra.Command, m *config.MountInstance) {
	if cookie, _ := cmd.Flags().GetString("cookie"); cookie != "" {
		m.Params.Cookie = cookie
	}
	if password, _ := cmd.Flags().GetString("password"); password != "" {
		if m.Encryption == nil {
			m.Encryption = &config.EncryptionConfig{Password: password}
		} else {
			m.Encryption.Password = password
		}
	}
	if salt, _ := cmd.Flags().GetString("salt"); salt != "" {
		if m.Encryption == nil {
			m.Encryption = &config.EncryptionConfig{Salt: salt}
		} else {
			m.Encryption.Salt = salt
		}
	}
	if mountPoint, _ := cmd.Flags().GetString("mount"); mountPoint != "" {
		m.MountPoint = config.ExpandHome(mountPoint)
	}
	if rootPath, _ := cmd.Flags().GetString("root-path"); rootPath != "" {
		m.Params.RootPath = rootPath
	}
}

func mountAllAndWait(targets []config.MountInstance, cfg *config.Config) {
	var tasks []mountTarget

	for _, m := range targets {
		host, vfs := setupMountInstance(m, cfg)
		if host == nil {
			continue
		}
		tasks = append(tasks, mountTarget{instance: m, host: host, vfs: vfs})
	}

	if len(tasks) == 0 {
		fmt.Println("所有挂载实例均初始化失败")
		os.Exit(1)
	}

	if len(tasks) == 1 {
		// Single mount — use original blocking pattern for cleaner signal handling
		t := tasks[0]
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)
		go func() {
			sig := <-sigChan
			switch sig {
			case syscall.SIGUSR1:
				buf := make([]byte, 1<<20)
				n := runtime.Stack(buf, true)
				log.L.Errorf("=== SIGUSR1: goroutine dump ===\n%s\n=== END dump ===\n", buf[:n])
				return
			default:
				fmt.Printf("\n正在关闭 %s...等待上传完成...\n", t.instance.Name)
				log.L.Infof("Shutdown: received signal, starting shutdown...\n")
			}
			t.vfs.Shutdown()
			fmt.Println("\n正在卸载...")
			t.host.Unmount()
			log.L.Infof("Shutdown complete\n")
			os.Exit(0)
		}()
		fmt.Printf("挂载 %s (%s) 到 %s... (Ctrl+C 卸载)\n", t.instance.Name, cfg.MergeInstanceConfig(t.instance).Type, config.ExpandHome(t.instance.MountPoint))
		t.host.Mount(config.ExpandHome(t.instance.MountPoint), fs.MountOptions(cfg.MergeInstanceConfig(t.instance).AllowOther))
		return
	}

	// Multiple mounts — use goroutine-per-mount with shared signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)

	var wg sync.WaitGroup
	for _, t := range tasks {
		wg.Add(1)
		go func(t mountTarget) {
			defer wg.Done()
			rc := cfg.MergeInstanceConfig(t.instance)
			t.host.Mount(config.ExpandHome(t.instance.MountPoint), fs.MountOptions(rc.AllowOther))
		}(t)
		fmt.Printf("挂载 %s (%s) 到 %s...\n", t.instance.Name, cfg.MergeInstanceConfig(t.instance).Type, config.ExpandHome(t.instance.MountPoint))
	}

	sig := <-sigChan
	if sig == syscall.SIGUSR1 {
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		log.L.Errorf("=== SIGUSR1: goroutine dump ===\n%s\n=== END dump ===\n", buf[:n])
		// Wait indefinitely for the user to send SIGINT
		<-sigChan
	}

	fmt.Println("\n正在关闭...等待上传完成...")
	log.L.Infof("Shutdown: received signal, starting shutdown...\n")

	for _, t := range tasks {
		t.vfs.Shutdown()
	}
	for _, t := range tasks {
		t.host.Unmount()
	}
	log.L.Infof("Shutdown complete\n")
}

func setupMountInstance(m config.MountInstance, cfg *config.Config) (*fuse.FileSystemHost, *fs.QryptFS) {
	rc := cfg.MergeInstanceConfig(m)

	cipher, err := config.MakeCipher(rc.Encryption, cfg.Defaults.Encryption, "", "")
	if err != nil {
		log.L.Errorf("加密引擎初始化失败: %v\n", err)
		fmt.Printf("  %s: 加密引擎初始化失败: %v\n", m.Name, err)
		return nil, nil
	}

	drv, err := factory.NewDriverFromType(rc.Type, rc.Params)
	if err != nil {
		fmt.Printf("  %s: 创建驱动失败: %v\n", m.Name, err)
		return nil, nil
	}
	if setter, ok := drv.(interface{ SetCipher(*crypt.RcloneCipher) }); ok {
		setter.SetCipher(cipher)
	}

	if err := drv.Init(context.Background()); err != nil {
		fmt.Printf("  %s: 认证失败: %v\n", m.Name, err)
		return nil, nil
	}

	rootFid := "0"
	if resolver, ok := drv.(interface{ ResolvePath(ctx context.Context, path string) (string, error) }); ok {
		rootPath := config.RootPathForMount(m)
		if rootPath != "" && rootPath != "/" {
			fmt.Printf("  解析路径: %s...\n", rootPath)
			fid, err := resolver.ResolvePath(context.Background(), rootPath)
			if err != nil {
				fmt.Printf("  %s: 解析路径失败: %v\n", m.Name, err)
				return nil, nil
			}
			rootFid = fid
		}
	} else if rc.Type == "yun139" && rc.Params.RootID != "" {
		rootFid = rc.Params.RootID
	}

	cacheMaxSize, err := config.ParseSize(rc.Cache.MaxSize)
	if err != nil {
		fmt.Printf("  解析缓存大小失败: %v，使用默认值 10GB\n", err)
		cacheMaxSize = 10 * 1024 * 1024 * 1024
	}
	cacheMgr, err := cache.NewCacheManager(rc.CacheDir, cacheMaxSize)
	if err != nil {
		fmt.Printf("  %s: 缓存初始化失败: %v\n", m.Name, err)
		return nil, nil
	}

	vfs := fs.NewFS(drv, cipher, cacheMgr, rootFid, fs.FSOptions{
		MaxRetries:        rc.Sync.MaxRetries,
		ConcurrentUploads: rc.Sync.ConcurrentUploads,
		MemCacheSizeMB:    rc.Cache.MemCacheSizeMB,
	})
	host := fuse.NewFileSystemHost(vfs)
	return host, vfs
}
