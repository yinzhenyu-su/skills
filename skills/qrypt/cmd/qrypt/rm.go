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

func runRm(cmd *cobra.Command, args []string) {
	socketPath := daemon.FindSocketPath()
	if !daemon.IsDaemonRunning(socketPath) {
		fmt.Println("错误: qryptd 未运行，请先启动 qryptd")
		fmt.Println("提示: 运行 qryptd 启动守护进程，以使用删除功能")
		os.Exit(1)
	}
	runRmViaDaemon(cmd, args, socketPath)
}

func runRmViaDaemon(cmd *cobra.Command, args []string, socketPath string) {
	path := args[0]
	mountName := resolveMount(cmd, &path)
	recursive, _ := cmd.Flags().GetBool("recursive")
	recursiveUpper, _ := cmd.Flags().GetBool("recursive-upper")
	isRecursive := recursive || recursiveUpper
	force, _ := cmd.Flags().GetBool("force")
	interactive, _ := cmd.Flags().GetBool("interactive")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	password, _ := cmd.Flags().GetString("password")
	salt, _ := cmd.Flags().GetString("salt")

	client, err := daemon.DialWS(socketPath)
	if err != nil {
		fmt.Printf("无法连接到 qryptd: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	for _, p := range args {
		path = p
		mountName = resolveMount(cmd, &path)

		if interactive {
			fmt.Printf("确认删除 %s? (y/N): ", path)
			reader := bufio.NewReader(os.Stdin)
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))
			if response != "y" && response != "yes" {
				fmt.Printf("已取消删除: %s\n", path)
				continue
			}
		}

		if dryRun {
			fmt.Printf("[Dry Run] 将要删除: %s\n", path)
			continue
		}

		resp, rpcErr := client.Call("remove", protocol.RemoveParams{
			MountName: mountName,
			Path:      path,
			Recursive: isRecursive,
			Force:     force,
			Password:  password,
			Salt:      salt,
		})
		if rpcErr != nil {
			fmt.Printf("RPC 错误: %v\n", rpcErr)
			os.Exit(1)
		}
		if resp.Error != nil {
			fmt.Printf("删除失败 (%s): %s\n", path, resp.Error.Message)
			os.Exit(1)
		}
		fmt.Printf("已删除: %s\n", path)
	}
}


