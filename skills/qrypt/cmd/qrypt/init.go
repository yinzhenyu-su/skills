package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

//go:embed .qrypt.example.toml
var exampleConfig string

func runInit(cmd *cobra.Command, args []string) {
	outputPath, _ := cmd.Flags().GetString("output")
	if _, err := os.Stat(outputPath); err == nil {
		fmt.Printf("文件已存在: %s\n", outputPath)
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		fmt.Printf("创建目录失败: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outputPath, []byte(exampleConfig), 0644); err != nil {
		fmt.Printf("写入配置文件失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("已生成配置文件: %s\n", outputPath)
}
