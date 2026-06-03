package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func runRm(cmd *cobra.Command, args []string) {
	api, err := apiFromCmd(cmd)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		os.Exit(1)
	}

	recursive, _ := cmd.Flags().GetBool("recursive")
	recursiveUpper, _ := cmd.Flags().GetBool("recursive-upper")
	isRecursive := recursive || recursiveUpper
	force, _ := cmd.Flags().GetBool("force")
	interactive, _ := cmd.Flags().GetBool("interactive")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

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

		err := api.Remove(context.Background(), mountName, path, isRecursive)
		if err != nil {
			if force {
				continue
			}
			fmt.Printf("删除失败 (%s): %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("已删除: %s\n", path)
	}
}
