package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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

	// ls 子命令 - 列出目录内容
	var lsCmd = &cobra.Command{
		Use:   "ls [path]",
		Short: "列出目录内容",
		Args:  cobra.MaximumNArgs(1),
		Run:   runList,
	}
	lsCmd.Flags().StringP("config", "f", "", "配置文件路径 (默认搜索 qrypt.toml)")
	lsCmd.Flags().BoolP("long", "l", false, "长格式显示（包含大小和时间）")
	lsCmd.Flags().BoolP("encrypted", "e", false, "同时显示加密后的文件名")

	// cat 子命令 - 解密并输出文件内容
	var catCmd = &cobra.Command{
		Use:   "cat <path>",
		Short: "解密并输出文件内容",
		Args:  cobra.ExactArgs(1),
		Run:   runCat,
	}
	catCmd.Flags().StringP("config", "f", "", "配置文件路径 (默认搜索 qrypt.toml)")

	// config 子命令 - 显示当前配置摘要
	var configCmd = &cobra.Command{
		Use:   "config",
		Short: "显示当前配置摘要",
		Run:   runConfig,
	}
	configCmd.Flags().StringP("config", "f", "", "配置文件路径 (默认搜索 qrypt.toml)")

	// status 子命令 - 显示运行状态和缓存统计
	var statusCmd = &cobra.Command{
		Use:   "status",
		Short: "显示缓存统计和运行状态",
		Run:   runStatus,
	}
	statusCmd.Flags().StringP("config", "f", "", "配置文件路径 (默认搜索 qrypt.toml)")

	rootCmd.AddCommand(mountCmd, initCmd, lsCmd, catCmd, configCmd, statusCmd)

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

	toolCmd.AddCommand(encryptCmd, decryptCmd, encSizeCmd)
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
	if err := d.Auth(); err != nil {
		fmt.Printf("认证失败: %v\n", err)
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

	fmt.Printf("已生成配置文件: %s\n", outputPath)
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

// -- ls --

func runList(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)

	d := driver.NewQuarkDriver(cfg.Quark.Cookie)
	d.SetCipher(cipher)
	if err := d.Auth(); err != nil {
		fmt.Printf("认证失败: %v\n", err)
		os.Exit(1)
	}

	path := "/"
	if len(args) > 0 {
		path = args[0]
	}
	fullPath := resolveFullPath(cfg.Quark.RootPath, path)

	fid, err := d.ResolvePath(fullPath)
	if err != nil {
		fmt.Printf("无法解析路径: %v\n", err)
		os.Exit(1)
	}

	files, err := d.ListFiles(fid)
	if err != nil {
		fmt.Printf("无法列出目录内容: %v\n", err)
		os.Exit(1)
	}

	showLong, _ := cmd.Flags().GetBool("long")
	showEnc, _ := cmd.Flags().GetBool("encrypted")

	for _, f := range files {
		decName, decErr := cipher.DecryptSegment(f.FileName)
		if decErr != nil {
			decName = f.FileName
		}

		if showLong {
			if f.IsDir() {
				if showEnc && f.FileName != decName {
					fmt.Printf("d %12s  %s  %s  [%s]\n", "-", f.ModTime().Format("01-02 15:04"), decName, f.FileName)
				} else {
					fmt.Printf("d %12s  %s  %s/\n", "-", f.ModTime().Format("01-02 15:04"), decName)
				}
			} else {
				plainSize, err := cipher.DecryptedSize(f.Int64Size())
				if err != nil {
					plainSize = f.Int64Size()
				}
				if showEnc && f.FileName != decName {
					fmt.Printf("- %10d  %s  %s  [%s]\n", plainSize, f.ModTime().Format("01-02 15:04"), decName, f.FileName)
				} else {
					fmt.Printf("- %10d  %s  %s\n", plainSize, f.ModTime().Format("01-02 15:04"), decName)
				}
			}
		} else {
			if f.IsDir() {
				fmt.Printf("%s/\n", decName)
			} else {
				fmt.Println(decName)
			}
		}
	}
}

// -- cat --

const (
	blocksPerSegment = 128 // 每个分段包含的加密块数 (~8MB 加密数据)
	catPrefetchDist  = 4   // 后台预取分段数（内存 ~ 5×8MB = 40MB 峰值）
)

// catSeg 代表一个下载完成的分段
type catSeg struct {
	idx  int
	data []byte
	err  error
}

func runCat(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)

	d := driver.NewQuarkDriver(cfg.Quark.Cookie)
	d.SetCipher(cipher)
	if err := d.Auth(); err != nil {
		fmt.Printf("认证失败: %v\n", err)
		os.Exit(1)
	}

	path := args[0]
	fullPath := resolveFullPath(cfg.Quark.RootPath, path)

	parentPath := filepath.Dir(fullPath)
	baseName := filepath.Base(fullPath)

	parentFid, err := d.ResolvePath(parentPath)
	if err != nil {
		fmt.Printf("无法解析路径: %v\n", err)
		os.Exit(1)
	}

	files, err := d.ListFiles(parentFid)
	if err != nil {
		fmt.Printf("无法列出目录内容: %v\n", err)
		os.Exit(1)
	}

	encName := cipher.EncryptSegment(baseName)
	var targetFile *driver.File
	for i := range files {
		if files[i].FileName == encName {
			targetFile = &files[i]
			break
		}
	}
	if targetFile == nil {
		fmt.Printf("文件未找到: %s\n", baseName)
		os.Exit(1)
	}
	if targetFile.IsDir() {
		fmt.Printf("错误: %s 是一个目录\n", baseName)
		os.Exit(1)
	}

	encSize := targetFile.Int64Size()
	fid := targetFile.Fid

	url, err := d.GetDownloadURL(fid)
	if err != nil {
		fmt.Printf("获取下载链接失败: %v\n", err)
		os.Exit(1)
	}

	// 1. 下载文件头获取 nonce（必须串行）
	rc, err := d.DownloadChunk(url, 0, int64(crypt.FileHeaderSize-1))
	if err != nil {
		fmt.Printf("下载文件头失败: %v\n", err)
		os.Exit(1)
	}
	header := make([]byte, crypt.FileHeaderSize)
	if _, err := io.ReadFull(rc, header); err != nil {
		rc.Close()
		fmt.Printf("读取文件头失败: %v\n", err)
		os.Exit(1)
	}
	rc.Close()

	if string(header[:len(crypt.FileMagic)]) != crypt.FileMagic {
		fmt.Fprintf(os.Stderr, "警告: 文件格式不是有效的 rclone 加密文件\n")
	}

	var fileNonce [crypt.FileNonceSize]byte
	copy(fileNonce[:], header[crypt.FileMagicSize:])

	bodySize := encSize - int64(crypt.FileHeaderSize)
	if bodySize <= 0 {
		return
	}

	// 2. 计算分段
	segEncBytes := int64(blocksPerSegment * crypt.BlockSize)
	numSegs := int((bodySize + segEncBytes - 1) / segEncBytes)

	// 3. 流水线预取：启动初始 catPrefetchDist 个异步下载
	type segFuture struct {
		ch  chan catSeg
		idx int
	}
	futures := make([]segFuture, 0, catPrefetchDist)

	prefetch := func(idx int) {
		ch := make(chan catSeg, 1)
		go func() {
			start := int64(crypt.FileHeaderSize) + int64(idx)*segEncBytes
			end := start + segEncBytes - 1
			if end >= encSize {
				end = encSize - 1
			}
			rc, err := d.DownloadChunk(url, start, end)
			if err != nil {
				ch <- catSeg{idx: idx, err: err}
				return
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				ch <- catSeg{idx: idx, err: err}
				return
			}
			ch <- catSeg{idx: idx, data: data}
		}()
		futures = append(futures, segFuture{ch: ch, idx: idx})
	}

	for i := 0; i < catPrefetchDist && i < numSegs; i++ {
		prefetch(i)
	}

	// 4. 按序处理，同时提交下一个预取任务
	nextPrefetch := catPrefetchDist
	for segIdx := 0; segIdx < numSegs; segIdx++ {
		if nextPrefetch < numSegs {
			prefetch(nextPrefetch)
			nextPrefetch++
		}

		r := <-futures[0].ch
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "\n下载分段 %d 失败: %v\n", r.idx, r.err)
			os.Exit(1)
		}

		processCatSegment(r.data, cipher, fileNonce, segIdx*blocksPerSegment)
		futures = futures[1:]
	}
}

// processCatSegment 解密并输出一个分段中的所有块
func processCatSegment(data []byte, cipher *crypt.RcloneCipher, fileNonce [crypt.FileNonceSize]byte, startBlockIdx int) {
	blockIdx := startBlockIdx
	for offset := 0; offset < len(data); {
		blockEnd := offset + crypt.BlockSize
		if blockEnd > len(data) {
			blockEnd = len(data)
		}
		encBlock := data[offset:blockEnd]
		offset = blockEnd

		if len(encBlock) <= crypt.BlockHeaderSize {
			break
		}

		plain, err := cipher.DecryptBlock(encBlock, uint64(blockIdx), fileNonce)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n解密块 %d 失败: %v\n", blockIdx, err)
			os.Exit(1)
		}

		if _, err := os.Stdout.Write(plain); err != nil {
			os.Exit(0)
		}
		blockIdx++
	}
}

// -- config --

func runConfig(cmd *cobra.Command, args []string) {
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

// -- status --

func runStatus(cmd *cobra.Command, args []string) {
	configPath, _ := cmd.Flags().GetString("config")
	if configPath == "" {
		configPath = config.FindConfigFile()
	}
	cfg, _ := config.LoadConfig(configPath)

	fmt.Println("=== Qrypt Status ===")

	// 配置信息
	fmt.Printf("配置文件:    %s\n", configPath)
	if configPath != "" {
		fmt.Printf("  缓存目录:  %s\n", cfg.Cache.Dir)
		fmt.Printf("  缓存上限:  %s\n", cfg.Cache.MaxSize)
		fmt.Printf("  挂载点:    %s\n", cfg.Mount.Point)
	}

	// 检查进程是否在运行
	fmt.Println()
	procRunning := checkQryptProcess()
	if procRunning {
		fmt.Println("运行状态:    运行中")
	} else {
		fmt.Print("运行状态:    ")
		fmt.Println("未运行")
	}

	// 检查挂载点
	fmt.Println()
	if cfg.Mount.Point != "" {
		mountPoint := config.ExpandHome(cfg.Mount.Point)
		if isMounted(mountPoint) {
			fmt.Printf("挂载状态:    已挂载到 %s\n", mountPoint)
		} else {
			fmt.Printf("挂载状态:    未挂载\n")
		}
	}

	// 打开缓存数据库
	dbPath := cfg.Cache.DBName
	if dbPath != "" {
		if _, err := os.Stat(dbPath); err == nil {
			db, err := cache.NewCacheDB(dbPath)
			if err == nil {
				info := db.GetStatusInfo()
				db.Close()

				// 缓存统计
				fmt.Println()
				fmt.Println("--- 缓存 ---")
				fmt.Printf("分块数:      %s\n", formatComma(info.ChunkCount))
				cacheMax, _ := config.ParseSize(cfg.Cache.MaxSize)
				fmt.Printf("已用空间:    %s / %s (%d%%)\n",
					formatBytes(info.ChunkTotalSize),
					cfg.Cache.MaxSize,
					percentOrZero(info.ChunkTotalSize, cacheMax))

				if info.ChunkOldestDays > 0 {
					fmt.Printf("最旧分块:    %.1f 天前\n", info.ChunkOldestDays)
				}

				// 待同步
				fmt.Println()
				fmt.Println("--- 待同步 ---")
				fmt.Printf("未完成上传:  %d 个\n", info.PendingNodeCount)
				if info.PendingNodeCount > 0 {
					nodes, _ := db.GetPendingNodes()
					for _, n := range nodes {
						fmt.Printf("  %s  (%s)\n", n.Path, formatBytes(n.Size))
					}
				}

				// 操作日志
				fmt.Println()
				fmt.Println("--- 操作日志 ---")
				fmt.Printf("待处理:      %d\n", info.OpsLogPending)
				fmt.Printf("已完成:      %d\n", info.OpsLogDone)
				if info.OpsLogFailed > 0 {
					fmt.Printf("失败:        %d\n", info.OpsLogFailed)
				}

				// Staging
				fmt.Println()
				fmt.Println("--- Staging ---")
				fmt.Printf("文件数:      %d\n", info.StagingFileCount)
				fmt.Printf("总大小:      %s\n", formatBytes(info.StagingTotalSize))
			}
		} else {
			fmt.Println()
			fmt.Println("--- 缓存 ---")
			fmt.Println("缓存数据库不存在（尚未挂载过）")
		}
	}
}

func checkQryptProcess() bool {
	cmd := exec.Command("pgrep", "-f", "qrypt mount")
	out, err := cmd.Output()
	return err == nil && len(out) > 0
}

func isMounted(point string) bool {
	// macOS: mount | grep fuse
	// Linux: mount | grep fuse
	cmd := exec.Command("mount")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), point) && strings.Contains(string(out), "fuse")
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

func formatComma(n int) string {
	s := fmt.Sprintf("%d", n)
	parts := make([]string, 0)
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return strings.Join(parts, ",")
}

func percentOrZero(a, b int64) int {
	if b == 0 {
		return 0
	}
	return int(a * 100 / b)
}

// resolveFullPath 将 rootPath 和用户路径拼接为完整路径
func resolveFullPath(rootPath, userPath string) string {
	root := strings.TrimRight(rootPath, "/")
	user := strings.TrimLeft(userPath, "/")
	if root == "" || root == "/" {
		return "/" + user
	}
	if user == "" {
		return root
	}
	return root + "/" + user
}
