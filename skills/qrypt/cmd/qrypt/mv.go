package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

func runMv(cmd *cobra.Command, args []string) {
	client, err := ensureDaemon()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	srcPath := args[0]
	dstArg := args[1]
	mountName := resolveMount(cmd, &srcPath)
	interactive, _ := cmd.Flags().GetBool("interactive")
	noClobber, _ := cmd.Flags().GetBool("no-clobber")
	password, _ := cmd.Flags().GetString("password")
	salt, _ := cmd.Flags().GetString("salt")

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

	resp, rpcErr := client.Call("move", protocol.MoveParams{
		MountName: mountName,
		SrcPath:   srcPath,
		DstPath:   dstArg,
		NoClobber: noClobber,
		Password:  password,
		Salt:      salt,
	})
	if rpcErr != nil {
		fmt.Printf("RPC 错误: %v\n", rpcErr)
		os.Exit(1)
	}
	if resp.Error != nil {
		fmt.Printf("移动失败: %s\n", resp.Error.Message)
		os.Exit(1)
	}
	fmt.Printf("已移动: %s → %s\n", srcPath, dstArg)
}
