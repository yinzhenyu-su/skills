package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func runMkdir(cmd *cobra.Command, args []string) {
	api, err := apiFromCmd(cmd)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}

	path := args[0]
	mountName := resolveMount(cmd, &path)

	err = api.Mkdir(context.Background(), mountName, path)
	if err != nil {
		fmt.Printf("创建目录失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("已创建: %s\n", path)
}
