package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/cache"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/driver"
	"github.com/yinzhenyu/skills/qrypt/internal/vfs"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "qrypt",
		Short: "Qrypt - Quark Drive Rclone-Compatible Crypt Mount Tool",
	}

	var mountCmd = &cobra.Command{
		Use:   "mount",
		Short: "Mount Quark Drive to a local directory",
		Run:   runMount,
	}

	// mount 子命令参数
	mountCmd.Flags().StringP("config", "f", "", "配置文件路径 (默认搜索 qrypt.toml)")
	mountCmd.Flags().StringP("cookie", "c", "", "Quark Drive Cookie")
	mountCmd.Flags().StringP("cache", "a", "", "本地缓存目录")
	mountCmd.Flags().StringP("mount", "m", "", "本地挂载点")
	mountCmd.Flags().StringP("password", "p", "", "Rclone 密码")
	mountCmd.Flags().StringP("salt", "s", "", "Rclone salt (可选)")
	mountCmd.Flags().StringP("root-path", "r", "", "Quark Drive 挂载路径")
	mountCmd.Flags().String("log-level", "", "日志级别: debug, info, warn, error (覆盖配置文件)")

	// init 子命令 - 生成示例配置文件
	var initCmd = &cobra.Command{
		Use:   "init",
		Short: "生成示例配置文件",
		Run:   runInit,
	}
	initCmd.Flags().StringP("output", "o", "qrypt.toml", "输出文件路径")

	rootCmd.AddCommand(mountCmd, initCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runMount(cmd *cobra.Command, args []string) {
	// 1. 加载配置文件
	configPath, _ := cmd.Flags().GetString("config")
	if configPath == "" {
		configPath = config.FindConfigFile()
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
		os.Exit(1)
	}

	// 2. 命令行参数覆盖配置文件
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

	// 3. 验证必填参数
	if cfg.Quark.Cookie == "" {
		fmt.Println("错误: 缺少 Quark Cookie (配置文件或 -c 参数)")
		os.Exit(1)
	}
	if cfg.Encryption.Password == "" {
		fmt.Println("错误: 缺少加密密码 (配置文件或 -p 参数)")
		os.Exit(1)
	}
	if cfg.Mount.Point == "" {
		fmt.Println("错误: 缺少挂载点 (配置文件或 -m 参数)")
		os.Exit(1)
	}

	// 3.5 初始化日志系统
	levelLogger, err := driver.NewLevelLogger(cfg.Log.Level, cfg.Log.File)
	if err != nil {
		fmt.Printf("日志初始化失败: %v\n", err)
		os.Exit(1)
	}
	driver.Log = levelLogger
	defer levelLogger.Close()

	// 4. 初始化加密引擎
	cipher, err := crypt.NewRcloneCipher(cfg.Encryption.Password, cfg.Encryption.Salt)
	if err != nil {
		fmt.Printf("加密引擎初始化失败: %v\n", err)
		os.Exit(1)
	}

	// 5. 初始化驱动并验证
	d := driver.NewQuarkDriver(cfg.Quark.Cookie)
	d.SetCipher(cipher) // 设置加密引擎以便解析路径
	if err := d.Auth(); err != nil {		fmt.Printf("认证失败: %v\n", err)
		os.Exit(1)
	}

	// 6. 解析根目录 FID
	rootFid := "0"
	if cfg.Quark.RootPath != "/" && cfg.Quark.RootPath != "" {
		fmt.Printf("解析路径: %s...\n", cfg.Quark.RootPath)
		fid, err := d.ResolvePath(cfg.Quark.RootPath)
		if err != nil {
			fmt.Printf("解析路径失败: %v\n", err)
			os.Exit(1)
		}
		rootFid = fid
	}
	fmt.Printf("根目录 FID: %s\n", rootFid)

	// 7. 解析缓存大小
	cacheMaxSize, err := config.ParseSize(cfg.Cache.MaxSize)
	if err != nil {
		fmt.Printf("解析缓存大小失败: %v，使用默认值 10GB\n", err)
		cacheMaxSize = 10 * 1024 * 1024 * 1024
	}

	// 8. 初始化缓存
	cm, err := cache.NewCacheManager(cfg.Cache.Dir, cfg.Cache.DBName, cacheMaxSize)
	if err != nil {
		fmt.Printf("缓存初始化失败: %v\n", err)
		os.Exit(1)
	}

	// 9. 配置驱动参数
	if dirCacheTTL, err := config.ParseDuration(cfg.Sync.DirCacheTTL); err == nil {
		d.DirCacheTTL = dirCacheTTL
	}

	// 提取挂载根目录名（用于重建被删除的根目录）
	rootDirName := filepath.Base(cfg.Quark.RootPath)
	if rootDirName == "/" || rootDirName == "." {
		rootDirName = ""
	}

	// 10. 挂载
	fs := vfs.NewQryptFS(d, cm, rootFid, rootDirName, cipher, vfs.QryptFSConfig{
		MaxRetries:        cfg.Sync.MaxRetries,
		ConcurrentUploads: cfg.Sync.ConcurrentUploads,
	})
	host := fuse.NewFileSystemHost(fs)

	options := vfs.MountOptions(cfg.Mount.AllowOther)
	fmt.Printf("挂载 Quark Drive 到 %s... (Ctrl+C 卸载)\n", cfg.Mount.Point)

	// 处理信号，优雅退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n正在卸载...")

		// 先尝试 fusermount -u（更可靠）
		unmountCmd := exec.Command("fusermount", "-u", cfg.Mount.Point)
		if err := unmountCmd.Run(); err != nil {
			// fusermount 失败，尝试 cgofuse 的 Unmount
			go func() {
				time.Sleep(2 * time.Second)
				fmt.Println("强制退出")
				os.Exit(0)
			}()
			host.Unmount()
		}
		os.Exit(0)
	}()

	host.Mount(cfg.Mount.Point, options)
}

func runInit(cmd *cobra.Command, args []string) {
	outputPath, _ := cmd.Flags().GetString("output")

	// 检查文件是否已存在
	if _, err := os.Stat(outputPath); err == nil {
		fmt.Printf("文件已存在: %s (覆盖？使用 --force)\n", outputPath)
		os.Exit(1)
	}

	// 生成示例配置
	exampleConfig := `# Qrypt 配置文件
# Quark Drive 加密挂载工具

[quark]
# 夸克网盘 Cookie（必填）
cookie = "你的Cookie"
# 挂载的网盘路径（默认根目录）
root_path = "/"

[encryption]
# 加密密码（必填，与 rclone crypt 兼容）
password = "你的密码"
# 加密盐（可选）
salt = ""

[cache]
# 缓存目录
dir = "~/.qrypt/cache"
# 数据库文件名
db_name = "qrypt_cache.db"
# 最大缓存大小 (支持 KB, MB, GB, TB)
max_size = "10GB"

[mount]
# 本地挂载点
point = "~/QryptMount"
# 允许其他用户访问（需要 /etc/fuse.conf 配置）
allow_other = false

[sync]
# 同步失败重试次数
max_retries = 3
# 并发上传数
concurrent_uploads = 3
# 目录列表缓存时间 (支持 s, m, h)
dir_cache_ttl = "5m"

[log]
# 日志级别: debug, info, warn, error
level = "info"
# 日志文件路径（空则输出到终端）
file = ""
`

	if err := os.WriteFile(outputPath, []byte(exampleConfig), 0644); err != nil {
		fmt.Printf("写入配置文件失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ 已生成配置文件: %s\n", outputPath)
	fmt.Println("\n下一步:")
	fmt.Println("  1. 编辑配置文件，填入你的 Cookie 和密码")
	fmt.Println("  2. 运行: qrypt mount -f qrypt.toml")
}
