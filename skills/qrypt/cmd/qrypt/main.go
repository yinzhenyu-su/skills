package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "qrypt",
		Short: "Qrypt - Quark Drive Rclone-Compatible Crypt Mount Tool",
	}

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

	var initCmd = &cobra.Command{
		Use:   "init",
		Short: "生成示例配置文件",
		Run:   runInit,
	}
	initCmd.Flags().StringP("output", "o", "qrypt.toml", "输出文件路径")

	var lsCmd = &cobra.Command{
		Use:   "ls [path]",
		Short: "列出目录内容",
		Args:  cobra.MaximumNArgs(1),
		Run:   runList,
	}
	// Pre-register help with no shorthand so cobra's InitDefaultHelpFlag
	// doesn't claim -h (POSIX ls uses -h for human-readable).
	lsCmd.Flags().Bool("help", false, "help for ls")
	lsCmd.Flags().StringP("config", "f", "", "配置文件路径")
	lsCmd.Flags().BoolP("long", "l", false, "长格式显示")
	lsCmd.Flags().BoolP("encrypted", "e", false, "同时显示加密文件名")
	lsCmd.Flags().BoolP("recursive", "R", false, "递归列出所有子目录")
	lsCmd.Flags().BoolP("human-readable", "h", false, "以可读格式显示大小 (与 -l 一起使用)")
	lsCmd.Flags().BoolP("sort-time", "t", false, "按修改时间排序 (最新在前)")
	lsCmd.Flags().BoolP("sort-size", "S", false, "按文件大小排序 (最大在前)")
	lsCmd.Flags().Bool("json", false, "以 JSON 格式输出")

	var catCmd = &cobra.Command{
		Use:   "cat <path>",
		Short: "解密并输出文件内容",
		Args:  cobra.ExactArgs(1),
		Run:   runCat,
	}
	catCmd.Flags().StringP("config", "f", "", "配置文件路径")

	var configCmd = &cobra.Command{
		Use:   "config",
		Short: "显示当前配置摘要",
		Run:   runConfig,
	}
	configCmd.Flags().StringP("config", "f", "", "配置文件路径")

	var statusCmd = &cobra.Command{
		Use:   "status",
		Short: "显示缓存统计和运行状态",
		Run:   runStatus,
	}
	statusCmd.Flags().StringP("config", "f", "", "配置文件路径")

	var rmCmd = &cobra.Command{
		Use:   "rm <path>",
		Short: "删除文件或目录",
		Args:  cobra.ExactArgs(1),
		Run:   runRm,
	}
	rmCmd.Flags().StringP("config", "f", "", "配置文件路径")
	rmCmd.Flags().BoolP("recursive", "r", false, "递归删除目录及其内容")
	rmCmd.Flags().BoolP("recursive-upper", "R", false, "等同于 -r")
	rmCmd.Flags().BoolP("force", "F", false, "强制删除，忽略不存在的文件，不提示") // Note: using F as shorthand since 'f' is config
	rmCmd.Flags().BoolP("interactive", "i", false, "每次删除前进行交互式确认")
	rmCmd.Flags().Bool("dry-run", false, "只打印将要删除的文件列表，不执行真实删除")

	var mvCmd = &cobra.Command{
		Use:   "mv <src> <dst>",
		Short: "移动或重命名文件",
		Args:  cobra.ExactArgs(2),
		Run:   runMv,
	}
	mvCmd.Flags().StringP("config", "f", "", "配置文件路径")
	mvCmd.Flags().BoolP("interactive", "i", false, "覆盖目标文件前提示")
	mvCmd.Flags().BoolP("no-clobber", "n", false, "不覆盖已存在的文件")

	var findCmd = &cobra.Command{
		Use:   "find [path] <pattern>",
		Short: "递归搜索文件名",
		Args:  cobra.RangeArgs(1, 2),
		Run:   runFind,
	}
	findCmd.Flags().StringP("config", "f", "", "配置文件路径")
	findCmd.Flags().Bool("glob", false, "Glob 模式匹配")
	findCmd.Flags().Bool("regex", false, "正则表达式匹配")
	findCmd.Flags().Bool("exact", false, "精确匹配（非子串）")
	findCmd.Flags().BoolP("case-sensitive", "s", false, "大小写敏感")
	findCmd.Flags().StringP("type", "t", "", "过滤类型: f=文件, d=目录")
	findCmd.Flags().Int("maxdepth", -1, "最大递归深度 (-1=不限)")
	findCmd.Flags().IntP("max", "n", 0, "匹配数量上限 (0=不限)")
	findCmd.Flags().Bool("json", false, "JSON 格式输出")
	findCmd.Flags().BoolP("count", "c", false, "只显示匹配数")
	findCmd.Flags().Int("workers", 1, "并发遍历协程数 (1-8)")
	findCmd.Flags().String("size", "", "按大小过滤 (例: +1M, -500K, 100B)")

	var pullCmd = &cobra.Command{
		Use:   "pull <remote> [local]",
		Short: "下载并解密文件到本地",
		Args:  cobra.RangeArgs(1, 2),
		Run:   runPull,
	}
	pullCmd.Flags().StringP("config", "f", "", "配置文件路径")
	pullCmd.Flags().BoolP("update", "u", false, "增量同步：跳过目标已存在且更新的文件")
	pullCmd.Flags().Int("transfers", 4, "并发传输文件数量")
	pullCmd.Flags().Bool("dry-run", false, "只打印将要下载的文件列表，不执行真实下载")

	var pushCmd = &cobra.Command{
		Use:   "push <local> [remote]",
		Short: "加密并上传本地文件到网盘",
		Args:  cobra.RangeArgs(1, 2),
		Run:   runPush,
	}
	pushCmd.Flags().StringP("config", "f", "", "配置文件路径")
	pushCmd.Flags().BoolP("update", "u", false, "跳过目标端大小一致的已存在文件")
	pushCmd.Flags().Int("transfers", 4, "并发传输文件数量")
	pushCmd.Flags().Bool("dry-run", false, "只打印将要上传的文件列表，不执行真实上传")

	rootCmd.AddCommand(mountCmd, initCmd, lsCmd, catCmd, configCmd, statusCmd, rmCmd, mvCmd, findCmd, pullCmd, pushCmd)

	var toolCmd = &cobra.Command{
		Use:   "tool",
		Short: "工具命令：加密/解密文件名、大小计算",
	}
	toolCmd.PersistentFlags().StringP("config", "f", "", "配置文件路径")
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
