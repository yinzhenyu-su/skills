package main

import (
	"context"
	"fmt"
	"os"
	pathpkg "path"
	"strings"

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

	parents, _ := cmd.Flags().GetBool("parents")

	if parents {
		segments := strings.Split(strings.Trim(path, "/"), "/")
		currentPath := "/"
		for _, seg := range segments {
			currentPath = pathpkg.Join(currentPath, seg)
			err = api.Mkdir(context.Background(), mountName, currentPath)
			if err != nil {
				fmt.Printf("创建目录失败: %v\n", err)
				os.Exit(1)
			}
		}
	} else {
		err = api.Mkdir(context.Background(), mountName, path)
		if err != nil {
			fmt.Printf("创建目录失败: %v\n", err)
			os.Exit(1)
		}
	}

	displayPath := path
	if mountCfg.Params["root_path"] != "/" && mountCfg.Params["root_path"] != "" {
		displayPath = StripRootPath(mountCfg.Params["root_path"], path)
	}
	fmt.Printf("已创建: %s\n", displayPath)
}
