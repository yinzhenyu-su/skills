package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func runMkdir(cmd *cobra.Command, args []string) {
	path := args[0]
	mountName := resolveMount(cmd, &path)

	cfgPath, _ := cmd.Flags().GetString("config")
	cfg, err := getCfg(cfgPath)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}
	mountCfg, err := resolveMountConfig(cfg, mountName)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}

	api, err := apiFromCmdForMount(cmd, mountName)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}

	err = api.Mkdir(context.Background(), mountName, path)
	if err != nil {
		fmt.Printf("创建目录失败: %v\n", err)
		os.Exit(1)
	}
	displayPath := path
	if mountCfg.Params.RootPath != "/" && mountCfg.Params.RootPath != "" {
		displayPath = StripRootPath(mountCfg.Params.RootPath, path)
	}
	fmt.Printf("已创建: %s\n", displayPath)
}
