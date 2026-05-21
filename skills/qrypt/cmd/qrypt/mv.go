package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/daemon"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

func runMv(cmd *cobra.Command, args []string) {
	socketPath := daemon.FindSocketPath()
	if !daemon.IsDaemonRunning(socketPath) {
		fmt.Println("错误: qryptd 未运行，请先启动 qryptd")
		fmt.Println("提示: 运行 qryptd 启动守护进程，以使用移动功能")
		os.Exit(1)
	}
	runMvViaDaemon(cmd, args, socketPath)
}

func runMvViaDaemon(cmd *cobra.Command, args []string, socketPath string) {
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

	client, err := daemon.DialWS(socketPath)
	if err != nil {
		fmt.Printf("无法连接到 qryptd: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

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


