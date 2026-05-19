package main

import (
	"fmt"
	"os"

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

	result := config.ValidateConfigFile(cfgPath)

	fmt.Println("=== Config Validation ===")
	fmt.Printf("File:   %s\n", result.FilePath)
	if result.Valid {
		fmt.Println("Result: VALID")
	} else {
		fmt.Println("Result: INVALID")
	}
	fmt.Println()

	for _, c := range result.Checks {
		switch c.Status {
		case "ok":
			fmt.Printf("  [OK]     %s: %s\n", c.Field, c.Message)
		case "warn":
			fmt.Printf("  [WARN]   %s: %s\n", c.Field, c.Message)
		case "error":
			fmt.Printf("  [ERROR]  %s: %s\n", c.Field, c.Message)
		}
	}

	if !result.Valid {
		os.Exit(1)
	}
}

func init() {
	var validateCmd = &cobra.Command{
		Use:   "validate [config-file]",
		Short: "Validate qrypt configuration file",
		Long:  "Validate a qrypt configuration file and print human-readable results.",
		Args:  cobra.MaximumNArgs(1),
		Run:   runValidate,
	}
	validateCmd.Flags().StringP("config", "f", "", "Config file path")
	rootCmd.AddCommand(validateCmd)
}
