package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func runCat(cmd *cobra.Command, args []string) {
	path := args[0]
	mountName := resolveMount(cmd, &path)

	api, err := apiFromCmdForMount(cmd, mountName)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}

	rc, err := api.Read(context.Background(), mountName, path)
	if err != nil {
		fmt.Printf("读取文件失败: %v\n", err)
		os.Exit(1)
	}
	defer rc.Close()

	if _, err := io.Copy(os.Stdout, rc); err != nil {
		fmt.Fprintf(os.Stderr, "读取错误: %v\n", err)
		os.Exit(1)
	}
}


