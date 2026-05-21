package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

func runValidate(cmd *cobra.Command, args []string) {
	cfgPath, _ := cmd.Flags().GetString("config")
	if cfgPath == "" && len(args) > 0 {
		cfgPath = args[0]
	}
	loadedPath, _, _, err := config.LoadConfigAuto(cfgPath)
	if err != nil {
		fmt.Printf("加载配置文件失败: %v\n", err)
		os.Exit(1)
	}

	result := config.ValidateConfigFile(loadedPath)

	errors := 0
	warns := 0
	for _, c := range result.Checks {
		switch c.Status {
		case "error":
			errors++
		case "warn":
			warns++
		}
	}

	fmt.Printf("配置文件: %s\n", result.FilePath)
	fmt.Printf("版本:     %s\n", config.CurrentVersion)

	if result.Valid {
		fmt.Printf("✓ 配置有效 · %d 检查项 · %d 错误 · %d 警告\n", len(result.Checks), errors, warns)
	} else {
		fmt.Printf("✗ 配置无效 · %d 检查项 · %d 错误 · %d 警告\n", len(result.Checks), errors, warns)
	}

	// Show non-OK checks only
	var hasDetail bool
	for _, c := range result.Checks {
		if c.Status != "ok" {
			if !hasDetail {
				fmt.Println()
				hasDetail = true
			}
			printCheck(c)
		}
	}

	if !result.Valid {
		os.Exit(1)
	}
}

func printCheck(c config.ValidationCheck) {
	switch c.Status {
	case "warn":
		fmt.Printf("  ⚠ %s\n", c.Message)
	case "error":
		fmt.Printf("  ✗ %s\n", c.Message)
	}
}

func init() {
	var validateCmd = &cobra.Command{
		Use:   "validate [config-file]",
		Short: "校验配置文件",
		Long:  "校验 qrypt 配置文件并显示详细的检查结果。",
		Args:  cobra.MaximumNArgs(1),
		Run:   runValidate,
	}
	validateCmd.Flags().StringP("config", "f", "", "配置文件路径")
	rootCmd.AddCommand(validateCmd)
}
