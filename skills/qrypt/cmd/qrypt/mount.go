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
	rootCmd.AddCommand(mountCmd)
}

func runMount(cmd *cobra.Command, args []string) {
	configPath, _ := cmd.Flags().GetString("config")
	if configPath == "" {
		configPath = config.FindConfigFile()
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
		os.Exit(1)
	}

	if driveType, _ := cmd.Flags().GetString("drive-type"); driveType != "" {
		cfg.Drive.Type = driveType
	}
	if cookie, _ := cmd.Flags().GetString("cookie"); cookie != "" {
		if cfg.Drive.Quark == nil {
			cfg.Drive.Quark = &config.QuarkOptions{}
		}
		cfg.Drive.Quark.Cookie = cookie
	}
	if password, _ := cmd.Flags().GetString("password"); password != "" {
		cfg.Encryption.Password = password
	}
	if salt, _ := cmd.Flags().GetString("salt"); salt != "" {
		cfg.Encryption.Salt = salt
	}
	if cacheDir, _ := cmd.Flags().GetString("cache"); cacheDir != "" {
		cfg.Cache.Dir = config.ExpandHome(cacheDir)
	}
	if mountPoint, _ := cmd.Flags().GetString("mount"); mountPoint != "" {
		cfg.Mount.Point = config.ExpandHome(mountPoint)
	}
	if rootPath, _ := cmd.Flags().GetString("root-path"); rootPath != "" {
		if cfg.Drive.Quark == nil {
			cfg.Drive.Quark = &config.QuarkOptions{}
		}
		cfg.Drive.Quark.RootPath = rootPath
	}
	if logLevel, _ := cmd.Flags().GetString("log-level"); logLevel != "" {
		cfg.Log.Level = logLevel
	}

	if cfg.Drive.Type == "quark" && (cfg.Drive.Quark == nil || cfg.Drive.Quark.Cookie == "") {
		fmt.Println("错误: 缺少 Quark Cookie")
		fmt.Println("  请通过以下方式之一设置：")
		fmt.Println("    1. 在配置文件中设置 [drive.quark] 或 [quark] 节的 cookie")
		fmt.Println("    2. 使用 -c <cookie> 命令行参数")
		fmt.Println("")
		fmt.Println("  Cookie 获取方法：登录 https://pan.quark.cn，F12 → Network → 任意请求头中复制 Cookie")
		os.Exit(1)
	}
	if cfg.Encryption.Password == "" {
		fmt.Println("错误: 缺少加密密码")
		fmt.Println("  请通过以下方式之一设置：")
		fmt.Println("    1. 在配置文件中设置 encryption.password")
		fmt.Println("    2. 使用 -p <password> 命令行参数")
		os.Exit(1)
	}
	if cfg.Mount.Point == "" {
		fmt.Println("错误: 缺少挂载点")
		fmt.Println("  请通过以下方式之一设置：")
		fmt.Println("    1. 在配置文件中设置 mount.point")
		fmt.Println("    2. 使用 -m <path> 命令行参数")
		fmt.Println("    3. 运行 qrypt init 生成配置文件模板")
		os.Exit(1)
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

	cipher, err := crypt.NewRcloneCipher(cfg.Encryption.Password, cfg.Encryption.Salt)
	if err != nil {
		log.L.Errorf("加密引擎初始化失败: %v\n", err)
		fmt.Printf("加密引擎初始化失败: %v\n", err)
		os.Exit(1)
	}

	drv, err := factory.NewDriverFromConfig(cfg.Drive)
	if err != nil {
		fmt.Printf("创建驱动失败: %v\n", err)
		os.Exit(1)
	}

	if err := drv.Init(context.Background()); err != nil {
		fmt.Printf("认证失败: %v\n", err)
		os.Exit(1)
	}

	rootFid := "0"
	if resolver, ok := drv.(interface{ ResolvePath(ctx context.Context, path string) (string, error) }); ok {
		rootPath := cfg.RootPath()
		if rootPath != "" && rootPath != "/" {
			fmt.Printf("解析路径: %s...\n", rootPath)
			fid, err := resolver.ResolvePath(context.Background(), rootPath)
			if err != nil {
				fmt.Printf("解析路径失败: %v\n", err)
				os.Exit(1)
			}
			rootFid = fid
		}
	} else if cfg.Drive.Type == "yun139" && cfg.Drive.Yun139 != nil && cfg.Drive.Yun139.RootID != "" {
		rootFid = cfg.Drive.Yun139.RootID
	}
	fmt.Printf("根目录 ID: %s\n", rootFid)

	cacheMaxSize, err := config.ParseSize(cfg.Cache.MaxSize)
	if err != nil {
		fmt.Printf("解析缓存大小失败: %v，使用默认值 10GB\n", err)
		cacheMaxSize = 10 * 1024 * 1024 * 1024
	}
	cacheMgr, err := cache.NewCacheManager(cfg.Cache.Dir, cacheMaxSize)
	if err != nil {
		fmt.Printf("缓存初始化失败: %v\n", err)
		os.Exit(1)
	}

	vfs := fs.NewFS(drv, cipher, cacheMgr, rootFid, fs.FSOptions{
		MaxRetries:        cfg.Sync.MaxRetries,
		ConcurrentUploads: cfg.Sync.ConcurrentUploads,
		MemCacheSizeMB:    cfg.Cache.MemCacheSizeMB,
	})
	host := fuse.NewFileSystemHost(vfs)
	options := fs.MountOptions(cfg.Mount.AllowOther)

	fmt.Printf("挂载 Quark Drive 到 %s... (Ctrl+C 卸载)\n", cfg.Mount.Point)

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

	host.Mount(cfg.Mount.Point, options)
}
