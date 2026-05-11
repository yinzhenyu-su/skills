package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
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

	// ---- tool 子命令组 ----
	var toolCmd = &cobra.Command{
		Use:   "tool",
		Short: "工具命令：加密/解密文件名、大小计算",
	}
	toolCmd.PersistentFlags().StringP("config", "f", "", "配置文件路径 (默认搜索 qrypt.toml)")
	toolCmd.PersistentFlags().String("password", "", "加密密码 (覆盖配置文件)")
	toolCmd.PersistentFlags().String("salt", "", "加密盐 (覆盖配置文件)")

	var encryptCmd = &cobra.Command{
		Use:   "encrypt <name>",
		Short: "计算文件名的加密形式",
		Args:  cobra.ExactArgs(1),
		Run:   runEncrypt,
	}

	var decryptCmd = &cobra.Command{
		Use:   "decrypt <name>",
		Short: "解密文件名",
		Args:  cobra.ExactArgs(1),
		Run:   runDecrypt,
	}

	var encSizeCmd = &cobra.Command{
		Use:   "enc-size <bytes>",
		Short: "计算加密后的文件大小",
		Args:  cobra.ExactArgs(1),
		Run:   runEncSize,
	}

	var configCmd = &cobra.Command{
		Use:   "config",
		Short: "显示当前配置摘要",
		Run:   runToolConfig,
	}

	toolCmd.AddCommand(encryptCmd, decryptCmd, encSizeCmd, configCmd)
	rootCmd.AddCommand(toolCmd)

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

	// 3.5 初始化日志系统（带轮转配置）
	rotate := &driver.LogRotateConfig{
		MaxSize:    cfg.Log.MaxSize,
		MaxBackups: cfg.Log.MaxBackups,
		MaxAge:     cfg.Log.MaxAge,
	}
	if cfg.Log.Compress != nil {
		rotate.Compress = *cfg.Log.Compress
	}
	levelLogger, err := driver.NewLevelLogger(cfg.Log.Level, cfg.Log.File, rotate)
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
		MemCacheSizeMB:    cfg.Cache.MemCacheSizeMB,
	})
	host := fuse.NewFileSystemHost(fs)

	options := vfs.MountOptions(cfg.Mount.AllowOther)
	fmt.Printf("挂载 Quark Drive 到 %s... (Ctrl+C 卸载)\n", cfg.Mount.Point)

	// 处理信号，优雅退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGUSR1)
	go func() {
		sig := <-sigChan
		switch sig {
		case syscall.SIGUSR1:
			// Dump all goroutine stacks to log for debugging hangs
			buf := make([]byte, 1<<20)
			n := runtime.Stack(buf, true)
			driver.Log.Errorf("=== SIGUSR1: goroutine dump ===\n%s\n=== END dump ===\n", buf[:n])
			return
		default:
			fmt.Println("\n正在关闭...等待上传完成...")
		}

		// 停止接收新任务，等待正在处理的上传完成
		fs.Shutdown()

		fmt.Println("\n正在卸载...")
		// 先尝试 fusermount -u（更可靠，仅 Linux）
		unmountCmd := exec.Command("fusermount", "-u", cfg.Mount.Point)
		if err := unmountCmd.Run(); err != nil {
			// fusermount 失败，尝试 cgofuse 的 Unmount
			go func() {
				time.Sleep(2 * time.Second)
				levelLogger.Close()
				fmt.Println("强制退出")
				os.Exit(0)
			}()
			host.Unmount()
		}
		levelLogger.Close()
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
cookie = ""
# 挂载的网盘路径（默认根目录）
root_path = "/Test"

[encryption]
# 加密密码（必填，与 rclone crypt 兼容）
password = ""
# 加密盐（可选）
salt = ""

[cache]
# 缓存目录
dir = "~/.qrypt/cache"
# 数据库文件名
db_name = "~/.qrypt/qrypt_cache.db"
# 最大缓存大小 (支持 KB, MB, GB, TB)
max_size = "10GB"

[mount]
# 本地挂载点
point = "~/Qrypt"
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
level = "debug"
# 日志文件路径（空则输出到终端）
file = "~/.qrypt/qrypt.log"
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

// -- 工具命令辅助 --

// loadToolCfg 从配置文件加载加密引擎（tool 命令专用，无 FUSE/驱动/缓存开销）
// 策略：先尝试加载配置文件，失败则回退到默认配置 + CLI 覆盖。
// 用户可通过 --password （或配置文件）提供密码。
func loadToolCfg(cmd *cobra.Command) (*config.Config, *crypt.RcloneCipher) {
	configPath, _ := cmd.Flags().GetString("config")
	explicitConfig := configPath != ""
	if configPath == "" {
		configPath = config.FindConfigFile()
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		if explicitConfig {
			fmt.Printf("加载配置文件失败: %v\n", err)
			os.Exit(1)
		}
		// 自动发现的配置文件解析失败，使用默认配置
		cfg = config.DefaultConfig()
	}

	if pwd, _ := cmd.Flags().GetString("password"); pwd != "" {
		cfg.Encryption.Password = pwd
	}
	if salt, _ := cmd.Flags().GetString("salt"); salt != "" {
		cfg.Encryption.Salt = salt
	}
	if cfg.Encryption.Password == "" {
		fmt.Println("错误: 缺少加密密码 (配置文件或 --password 参数)")
		os.Exit(1)
	}

	cipher, err := crypt.NewRcloneCipher(cfg.Encryption.Password, cfg.Encryption.Salt)
	if err != nil {
		fmt.Printf("加密引擎初始化失败: %v\n", err)
		os.Exit(1)
	}
	return cfg, cipher
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

// -- encrypt --

func runEncrypt(cmd *cobra.Command, args []string) {
	_, cipher := loadToolCfg(cmd)
	name := args[0]
	encName := cipher.EncryptSegment(name)
	fmt.Printf("明文:  %s\n加密:  %s\n", name, encName)
}

// -- decrypt --

func runDecrypt(cmd *cobra.Command, args []string) {
	_, cipher := loadToolCfg(cmd)
	encName := args[0]
	plain, err := cipher.DecryptSegment(encName)
	if err != nil {
		fmt.Printf("解密失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("加密:  %s\n明文:  %s\n", encName, plain)
}

// -- enc-size --

func runEncSize(cmd *cobra.Command, args []string) {
	_, cipher := loadToolCfg(cmd)
	size, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		fmt.Printf("无效大小: %s\n", args[0])
		os.Exit(1)
	}
	encSize := cipher.EncryptedSize(size)
	fmt.Printf("明文大小:  %d\n加密大小:  %d\n", size, encSize)
}

// -- config --

func runToolConfig(cmd *cobra.Command, args []string) {
	cfg, _ := loadToolCfg(cmd)
	fmt.Println("=== Qrypt 配置 ===")
	fmt.Printf("Quark 根路径: %s\n", cfg.Quark.RootPath)
	fmt.Printf("缓存目录:     %s\n", cfg.Cache.Dir)
	fmt.Printf("挂载点:       %s\n", cfg.Mount.Point)
	fmt.Printf("加密密码:     %s\n", maskStr(cfg.Encryption.Password))
	if cfg.Encryption.Salt != "" {
		fmt.Printf("加密盐:       %s\n", cfg.Encryption.Salt)
	}
	fmt.Printf("日志级别:     %s\n", cfg.Log.Level)
	fmt.Printf("并发上传:     %d\n", cfg.Sync.ConcurrentUploads)
	fmt.Printf("缓存上限:     %s\n", cfg.Cache.MaxSize)
}
