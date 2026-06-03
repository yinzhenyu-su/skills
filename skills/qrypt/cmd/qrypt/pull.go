package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func runPull(cmd *cobra.Command, args []string) {
	api, err := apiFromCmd(cmd)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}

	remotePath := args[0]
	mountName := resolveMount(cmd, &remotePath)
	localPath := ""
	if len(args) >= 2 {
		localPath = args[1]
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if dryRun {
		fmt.Printf("[Dry Run] 将下载: %s → %s\n", remotePath, localPath)
		return
	}

	fmt.Printf("开始下载: %s\n", remotePath)
	err = api.Pull(context.Background(), mountName, remotePath, localPath)
	if err != nil {
		fmt.Printf("下载失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("下载完成\n")
}
