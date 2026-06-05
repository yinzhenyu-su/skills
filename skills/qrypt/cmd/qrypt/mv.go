package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func runMv(cmd *cobra.Command, args []string) {
	srcPath := args[0]
	dstArg := args[1]

	srcMount := resolveMount(cmd, &srcPath)
	dstMount := resolveMount(cmd, &dstArg)

	var mountName string
	switch {
	case srcMount != "" && dstMount != "":
		if srcMount != dstMount {
			fmt.Printf("错误: 源路径 (%s) 和目标路径 (%s) 必须属于同一挂载实例\n", srcMount, dstMount)
			os.Exit(1)
		}
		mountName = srcMount
	case srcMount != "":
		mountName = srcMount
	case dstMount != "":
		mountName = dstMount
	default:
		mountName = ""
	}

	api, err := apiFromCmdForMount(cmd, mountName)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}

	cfgPath, _ := cmd.Flags().GetString("config")
	cfg, err := getCfg(cfgPath)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}
	mountCfg, err := resolveMountConfig(cfg, mountName)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}
	rootPath := mountCfg.Params["root_path"]

	interactive, _ := cmd.Flags().GetBool("interactive")

	if interactive {
		fmt.Printf("确认移动 %s → %s? (y/N): ", srcPath, dstArg)
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Printf("已取消移动: %s\n", srcPath)
			return
		}
	}

	err = api.Move(context.Background(), mountName, srcPath, dstArg)
	if err != nil {
		fmt.Printf("移动失败: %v\n", err)
		os.Exit(1)
	}

	displaySrc := StripRootPath(rootPath, srcPath)
	displayDst := StripRootPath(rootPath, dstArg)
	fmt.Printf("已移动: %s → %s\n", displaySrc, displayDst)
}
