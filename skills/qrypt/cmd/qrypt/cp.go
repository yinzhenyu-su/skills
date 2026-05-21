package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/daemon"
	"github.com/yinzhenyu/skills/qrypt/internal/transfer"
)

func init() {
	var cpCmd = &cobra.Command{
		Use:   "cp <src> <dst>",
		Short: "跨网盘复制文件 (src_mount:path dst_mount:path)",
		Args:  cobra.ExactArgs(2),
		Run:   runCp,
	}
	cpCmd.Flags().StringP("config", "f", "", "配置文件路径")
	rootCmd.AddCommand(cpCmd)
}

func runCp(cmd *cobra.Command, args []string) {
	srcArg := args[0]
	dstArg := args[1]

	srcMount, srcPath := ParseMountPath(srcArg)
	dstMount, dstPath := ParseMountPath(dstArg)

	if srcMount == "" || dstMount == "" {
		fmt.Println("错误: cp 需要 mount_name:path 格式的两个参数")
		fmt.Println("示例: qrypt cp personal:/docs/file.txt work:/backup/")
		os.Exit(1)
	}

	cfg := loadToolCfgOnly(cmd)
	mm := daemon.NewMountManagerStandalone(cfg)

	var startedSrc, startedDst bool

	if _, err := mm.Get(srcMount); err != nil {
		if err := mm.Start(context.Background(), srcMount); err != nil {
			fmt.Printf("启动源挂载 %q 失败: %v\n", srcMount, err)
			os.Exit(1)
		}
		startedSrc = true
	}
	defer func() {
		if startedSrc {
			mm.Stop(context.Background(), srcMount)
		}
	}()

	if _, err := mm.Get(dstMount); err != nil {
		if err := mm.Start(context.Background(), dstMount); err != nil {
			fmt.Printf("启动目标挂载 %q 失败: %v\n", dstMount, err)
			os.Exit(1)
		}
		startedDst = true
	}
	defer func() {
		if startedDst {
			mm.Stop(context.Background(), dstMount)
		}
	}()

	tm := transfer.NewManager(mm)
	if err := tm.Copy(context.Background(), srcMount, srcPath, dstMount, dstPath); err != nil {
		fmt.Printf("复制失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("完成: %s:%s → %s:%s\n", srcMount, srcPath, dstMount, dstPath)
}
