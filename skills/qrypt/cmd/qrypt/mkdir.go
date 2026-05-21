package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

func runMkdir(cmd *cobra.Command, args []string) {
	client, err := ensureDaemon()
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	path := args[0]
	mountName := resolveMount(cmd, &path)
	parents, _ := cmd.Flags().GetBool("parents")
	password, _ := cmd.Flags().GetString("password")
	salt, _ := cmd.Flags().GetString("salt")

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
