package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

func runRm(cmd *cobra.Command, args []string) {
	client, err := ensureDaemon()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	recursive, _ := cmd.Flags().GetBool("recursive")
	recursiveUpper, _ := cmd.Flags().GetBool("recursive-upper")
	isRecursive := recursive || recursiveUpper
	force, _ := cmd.Flags().GetBool("force")
	interactive, _ := cmd.Flags().GetBool("interactive")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	password, _ := cmd.Flags().GetString("password")
	salt, _ := cmd.Flags().GetString("salt")

	for _, p := range args {
		path := p
		mountName := resolveMount(cmd, &path)

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
