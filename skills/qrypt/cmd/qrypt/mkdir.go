package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/daemon"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

func runMkdir(cmd *cobra.Command, args []string) {
	socketPath := daemon.FindSocketPath()
	if !daemon.IsDaemonRunning(socketPath) {
		fmt.Println("错误: qryptd 未运行，请先启动 qryptd")
		fmt.Println("提示: 运行 qryptd 启动守护进程，以使用创建目录功能")
		os.Exit(1)
	}
	runMkdirViaDaemon(cmd, args, socketPath)
}

func runMkdirViaDaemon(cmd *cobra.Command, args []string, socketPath string) {
	path := args[0]
	mountName := resolveMount(cmd, &path)
	parents, _ := cmd.Flags().GetBool("parents")
	password, _ := cmd.Flags().GetString("password")
	salt, _ := cmd.Flags().GetString("salt")

	client, err := daemon.DialWS(socketPath)
	if err != nil {
		fmt.Printf("无法连接到 qryptd: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	resp, rpcErr := client.Call("mkdir", protocol.MkdirParams{
		MountName: mountName,
		Path:      path,
		Parents:   parents,
		Password:  password,
		Salt:      salt,
	})
	if rpcErr != nil {
		fmt.Printf("RPC 错误: %v\n", rpcErr)
		os.Exit(1)
	}
	if resp.Error != nil {
		fmt.Printf("创建目录失败: %s\n", resp.Error.Message)
		os.Exit(1)
	}
	fmt.Printf("已创建: %s\n", path)
}


