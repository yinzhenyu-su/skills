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
	api, err := apiFromCmd(cmd)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}

	srcPath := args[0]
	dstArg := args[1]
	mountName := resolveMount(cmd, &srcPath)
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
	fmt.Printf("已移动: %s → %s\n", srcPath, dstArg)
}
