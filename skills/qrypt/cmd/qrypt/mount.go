package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/fs"
	"github.com/yinzhenyu/skills/qrypt/internal/log"
	"github.com/yinzhenyu/skills/qrypt/internal/quark"
)

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

	if cookie, _ := cmd.Flags().GetString("cookie"); cookie != "" {
		cfg.Quark.Cookie = cookie
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
		cfg.Quark.RootPath = rootPath
	}
	if logLevel, _ := cmd.Flags().GetString("log-level"); logLevel != "" {
		cfg.Log.Level = logLevel
	}

	if cfg.Quark.Cookie == "" {
		fmt.Println("错误: 缺少 Quark Cookie")
		fmt.Println("  请通过以下方式之一设置：")
		fmt.Println("    1. 在配置文件中设置 quark.cookie")
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

	quarkClient := quark.NewClient(cfg.Quark.Cookie)
	cacheSvc := quark.NewCacheService()
	if cacheTTL, err := config.ParseDuration(cfg.Sync.DirCacheTTL); err == nil {
		cacheSvc.DirCacheTTL = cacheTTL
	}
	fileSvc := quark.NewFileService(quarkClient, cacheSvc, cipher)
	manageSvc := quark.NewManageService(quarkClient)

	if err := fileSvc.Auth(); err != nil {
		fmt.Printf("认证失败: %v\n", err)
		os.Exit(1)
	}

	rootFid := "0"
	if cfg.Quark.RootPath != "/" && cfg.Quark.RootPath != "" {
		fmt.Printf("解析路径: %s...\n", cfg.Quark.RootPath)
		fid, err := fileSvc.ResolvePath(cfg.Quark.RootPath)
		if err != nil {
			fmt.Printf("解析路径失败: %v\n", err)
			os.Exit(1)
		}
		rootFid = fid
	}
	fmt.Printf("根目录 FID: %s\n", rootFid)

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

	vfs := fs.NewFS(fileSvc, manageSvc, cacheSvc, cipher, cacheMgr, quarkClient, rootFid, fs.FSOptions{
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
			log.L.Info("Shutdown: received signal, starting shutdown...\n")
		}

		vfs.Shutdown()

		fmt.Println("\n正在卸载...")
		go func() {
			time.Sleep(3 * time.Second)
			log.L.Info("Force exit\n")
			os.Exit(0)
		}()
		host.Unmount()
		log.L.Info("Shutdown complete\n")
		os.Exit(0)
	}()

	host.Mount(cfg.Mount.Point, options)
}
