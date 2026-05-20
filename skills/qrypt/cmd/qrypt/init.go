package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

func runInit(cmd *cobra.Command, args []string) {
	outputPath, _ := cmd.Flags().GetString("output")
	if err := config.WriteDefaultConfig(outputPath); err != nil {
		fmt.Printf("生成配置文件失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("已生成配置文件: %s\n", outputPath)
}
