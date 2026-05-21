//go:build cgo && !nofuse

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
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
		Use:   "mount",
		Short: "Mount cloud drive to a local directory",
		Run:   runMount,
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
	configPath, _ := cmd.Flags().GetString("config")
	if configPath == "" {
		configPath = config.FindConfigFile()
	}

	cfg, vr, err := config.LoadConfig(configPath)
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

	if len(cfg.Mounts) == 0 {
		fmt.Println("配置中没有挂载实例")
		os.Exit(1)
	}
	m := cfg.Mounts[0]

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
	if logLevel, _ := cmd.Flags().GetString("log-level"); logLevel != "" {
		cfg.Log.Level = logLevel
	}

	rc := cfg.MergeInstanceConfig(m)

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

	cipher, err := crypt.NewRcloneCipher(rc.Encryption.Password, rc.Encryption.Salt, rc.Encryption.FileNameEncoding)
	if err != nil {
		log.L.Errorf("加密引擎初始化失败: %v\n", err)
		fmt.Printf("加密引擎初始化失败: %v\n", err)
		os.Exit(1)
	}

	drv, err := factory.NewDriverFromType(rc.Type, rc.Params)
	if err != nil {
		fmt.Printf("创建驱动失败: %v\n", err)
		os.Exit(1)
	}
	if setter, ok := drv.(interface{ SetCipher(*crypt.RcloneCipher) }); ok {
		setter.SetCipher(cipher)
	}

	if err := drv.Init(context.Background()); err != nil {
		fmt.Printf("认证失败: %v\n", err)
		os.Exit(1)
	}

	rootFid := "0"
	if resolver, ok := drv.(interface{ ResolvePath(ctx context.Context, path string) (string, error) }); ok {
		rootPath := config.RootPathForMount(m)
		if rootPath != "" && rootPath != "/" {
			fmt.Printf("解析路径: %s...\n", rootPath)
			fid, err := resolver.ResolvePath(context.Background(), rootPath)
			if err != nil {
				fmt.Printf("解析路径失败: %v\n", err)
				os.Exit(1)
			}
			rootFid = fid
		}
	} else if rc.Type == "yun139" && rc.Params.RootID != "" {
		rootFid = rc.Params.RootID
	}
	fmt.Printf("根目录 ID: %s\n", rootFid)

	cacheMaxSize, err := config.ParseSize(rc.Cache.MaxSize)
	if err != nil {
		fmt.Printf("解析缓存大小失败: %v，使用默认值 10GB\n", err)
		cacheMaxSize = 10 * 1024 * 1024 * 1024
	}
	cacheMgr, err := cache.NewCacheManager(rc.CacheDir, cacheMaxSize)
	if err != nil {
		fmt.Printf("缓存初始化失败: %v\n", err)
		os.Exit(1)
	}

	vfs := fs.NewFS(drv, cipher, cacheMgr, rootFid, fs.FSOptions{
		MaxRetries:        rc.Sync.MaxRetries,
		ConcurrentUploads: rc.Sync.ConcurrentUploads,
		MemCacheSizeMB:    rc.Cache.MemCacheSizeMB,
	})
	host := fuse.NewFileSystemHost(vfs)
	options := fs.MountOptions(rc.AllowOther)

	fmt.Printf("挂载 %s (%s) 到 %s... (Ctrl+C 卸载)\n", rc.Name, rc.Type, rc.MountPoint)

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
			fmt.Println("\n正在关闭...等待上传完成...")
			log.L.Infof("Shutdown: received signal, starting shutdown...\n")
		}

		vfs.Shutdown()

		fmt.Println("\n正在卸载...")
		host.Unmount()
		log.L.Infof("Shutdown complete\n")
		os.Exit(0)
	}()

	host.Mount(rc.MountPoint, options)
}
