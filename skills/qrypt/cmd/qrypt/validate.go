package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

func runValidate(cmd *cobra.Command, args []string) {
	var cfgPath string
	if len(args) > 0 {
		cfgPath = args[0]
	}
	if cfgPath == "" {
		cfgPath, _ = cmd.Flags().GetString("config")
	}
	if cfgPath == "" {
		cfgPath = config.FindConfigFile()
	}
	if cfgPath == "" {
		fmt.Println("未找到配置文件")
		os.Exit(1)
	}

	result := config.ValidateConfigFile(cfgPath)

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
	fmt.Println()

	if result.Valid {
		fmt.Println("✓ 配置有效")
	} else {
		fmt.Println("✗ 配置无效")
	}
	fmt.Printf("  检查项 %d | 错误 %d | 警告 %d\n", len(result.Checks), errors, warns)
	fmt.Println()

	cm := result.ChecksMap()

	// Group: per-mount checks
	mountIndices := make(map[string]bool)
	for field := range cm {
		if strings.HasPrefix(field, "mounts[") {
			parts := strings.SplitN(field, ".", 2)
			mountIndices[parts[0]] = true
		}
	}

	for idx := range mountIndices {
		name := idx
		for _, c := range result.Checks {
			if c.Field == idx+".name" && c.Status == "ok" {
				name = c.Message
			}
		}
		fmt.Printf("── 挂载 %s ──\n", name)
		for _, c := range result.Checks {
			if strings.HasPrefix(c.Field, idx+".") {
				printCheck(c)
			}
		}
		fmt.Println()
	}

	// Defaults checks
	var hasDefaults bool
	for _, c := range result.Checks {
		if strings.HasPrefix(c.Field, "defaults") {
			if !hasDefaults {
				fmt.Println("── 全局默认值 ──")
				hasDefaults = true
			}
			printCheck(c)
		}
	}
	if hasDefaults {
		fmt.Println()
	}

	// Other checks (log, etc.)
	var hasOthers bool
	for _, c := range result.Checks {
		if !strings.HasPrefix(c.Field, "mounts[") && !strings.HasPrefix(c.Field, "defaults") {
			if !hasOthers {
				fmt.Println("── 其他 ──")
				hasOthers = true
			}
			printCheck(c)
		}
	}
	if hasOthers {
		fmt.Println()
	}

	if !result.Valid {
		os.Exit(1)
	}
}

func printCheck(c config.ValidationCheck) {
	switch c.Status {
	case "ok":
		fmt.Printf("  ✓ %s\n", c.Message)
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
