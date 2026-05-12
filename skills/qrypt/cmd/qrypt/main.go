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
		Short: "Mount Quark Drive to a local directory",
		Run:   runMount,
	}
	mountCmd.Flags().StringP("config", "f", "", "配置文件路径 (默认搜索 qrypt.toml)")
	mountCmd.Flags().StringP("cookie", "c", "", "Quark Drive Cookie")
	mountCmd.Flags().StringP("cache", "a", "", "本地缓存目录")
	mountCmd.Flags().StringP("mount", "m", "", "本地挂载点")
	mountCmd.Flags().StringP("password", "p", "", "Rclone 密码")
	mountCmd.Flags().StringP("salt", "s", "", "Rclone salt (可选)")
	mountCmd.Flags().StringP("root-path", "r", "", "Quark Drive 挂载路径")
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
	lsCmd.Flags().StringP("config", "f", "", "配置文件路径")
	lsCmd.Flags().BoolP("long", "l", false, "长格式显示")
	lsCmd.Flags().BoolP("encrypted", "e", false, "同时显示加密文件名")

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

	var mvCmd = &cobra.Command{
		Use:   "mv <src> <dst>",
		Short: "移动或重命名文件",
		Args:  cobra.ExactArgs(2),
		Run:   runMv,
	}
	mvCmd.Flags().StringP("config", "f", "", "配置文件路径")

	var findCmd = &cobra.Command{
		Use:   "find [path] <pattern>",
		Short: "递归搜索文件名",
		Args:  cobra.RangeArgs(1, 2),
		Run:   runFind,
	}
	findCmd.Flags().StringP("config", "f", "", "配置文件路径")

	var pullCmd = &cobra.Command{
		Use:   "pull <remote> [local]",
		Short: "下载并解密文件到本地",
		Args:  cobra.RangeArgs(1, 2),
		Run:   runPull,
	}
	pullCmd.Flags().StringP("config", "f", "", "配置文件路径")

	var pushCmd = &cobra.Command{
		Use:   "push <local> [remote]",
		Short: "加密并上传本地文件到网盘",
		Args:  cobra.RangeArgs(1, 2),
		Run:   runPush,
	}
	pushCmd.Flags().StringP("config", "f", "", "配置文件路径")

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
