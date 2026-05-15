package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func runList(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)
	drv := loadToolDriver(cfg, cipher)
	resolver, _ := drv.(pathResolver)

	path := "/"
	if len(args) > 0 {
		path = args[0]
	}
	fullPath := resolveFullPath(cfg.RootPath(), path)

	fid, err := resolver.ResolvePath(context.Background(), fullPath)
	if err != nil {
		fmt.Printf("无法解析路径: %v\n", err)
		os.Exit(1)
	}

	entries, err := drv.List(context.Background(), fid)
	if err != nil {
		fmt.Printf("无法列出目录内容: %v\n", err)
		os.Exit(1)
	}

	showLong, _ := cmd.Flags().GetBool("long")
	showEnc, _ := cmd.Flags().GetBool("encrypted")

	for _, e := range entries {
		decName, decErr := cipher.DecryptSegment(e.Name)
		if decErr != nil {
			decName = e.Name
		}

		if showLong {
			if e.IsDir {
				if showEnc && e.Name != decName {
					fmt.Printf("d %12s  %s  %s  [%s]\n", "-", e.ModTime.Format("01-02 15:04"), decName, e.Name)
				} else {
					fmt.Printf("d %12s  %s  %s/\n", "-", e.ModTime.Format("01-02 15:04"), decName)
				}
			} else {
				plainSize, err := cipher.DecryptedSize(e.Size)
				if err != nil {
					plainSize = e.Size
				}
				if showEnc && e.Name != decName {
					fmt.Printf("- %10d  %s  %s  [%s]\n", plainSize, e.ModTime.Format("01-02 15:04"), decName, e.Name)
				} else {
					fmt.Printf("- %10d  %s  %s\n", plainSize, e.ModTime.Format("01-02 15:04"), decName)
				}
			}
		} else {
			if e.IsDir {
				fmt.Printf("%s/\n", decName)
			} else {
				fmt.Println(decName)
			}
		}
	}
}
