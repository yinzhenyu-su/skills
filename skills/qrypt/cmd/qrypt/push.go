package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func runPush(cmd *cobra.Command, args []string) {
	localPath := args[0]
	remotePath := ""
	if len(args) >= 2 {
		remotePath = args[1]
	}
	mountName := resolveMount(cmd, &remotePath)

	api, err := apiFromCmdForMount(cmd, mountName)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")

	isStdin := localPath == "-"
	var tmpFile string

	if isStdin {
		f, err := os.CreateTemp("", "qrypt-stdin-*")
		if err != nil {
			fmt.Printf("创建临时文件失败: %v\n", err)
			os.Exit(1)
		}
		_, cpErr := io.Copy(f, os.Stdin)
		if cpErr != nil {
			f.Close()
			os.Remove(f.Name())
			fmt.Printf("读取标准输入失败: %v\n", cpErr)
			os.Exit(1)
		}
		f.Close()
		tmpFile = f.Name()
		localPath = tmpFile
	}

	if dryRun {
		fmt.Printf("[Dry Run] 将上传: %s → %s\n", localPath, remotePath)
		if tmpFile != "" {
			os.Remove(tmpFile)
		}
		return
	}

	fmt.Printf("开始推送: %s → %s\n", localPath, remotePath)
	err = api.Push(context.Background(), mountName, localPath, remotePath)
	if tmpFile != "" {
		os.Remove(tmpFile)
	}
	if err != nil {
		fmt.Printf("推送失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("推送完成\n")
}
