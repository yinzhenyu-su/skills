package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/quark"
)

func runList(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)
	quarkClient := quark.NewClient(cfg.Quark.Cookie)
	cacheSvc := quark.NewCacheService()
	fileSvc := quark.NewFileService(quarkClient, cacheSvc, cipher)

	if err := fileSvc.Auth(); err != nil {
		fmt.Printf("认证失败: %v\n", err)
		os.Exit(1)
	}

	path := "/"
	if len(args) > 0 {
		path = args[0]
	}
	fullPath := resolveFullPath(cfg.Quark.RootPath, path)

	fid, err := fileSvc.ResolvePath(fullPath)
	if err != nil {
		fmt.Printf("无法解析路径: %v\n", err)
		os.Exit(1)
	}

	files, err := fileSvc.ListFiles(fid)
	if err != nil {
		fmt.Printf("无法列出目录内容: %v\n", err)
		os.Exit(1)
	}

	showLong, _ := cmd.Flags().GetBool("long")
	showEnc, _ := cmd.Flags().GetBool("encrypted")

	for _, f := range files {
		decName, decErr := cipher.DecryptSegment(f.FileName)
		if decErr != nil {
			decName = f.FileName
		}

		if showLong {
			if f.IsDir() {
				if showEnc && f.FileName != decName {
					fmt.Printf("d %12s  %s  %s  [%s]\n", "-", f.ModTime().Format("01-02 15:04"), decName, f.FileName)
				} else {
					fmt.Printf("d %12s  %s  %s/\n", "-", f.ModTime().Format("01-02 15:04"), decName)
				}
			} else {
				plainSize, err := cipher.DecryptedSize(f.Int64Size())
				if err != nil {
					plainSize = f.Int64Size()
				}
				if showEnc && f.FileName != decName {
					fmt.Printf("- %10d  %s  %s  [%s]\n", plainSize, f.ModTime().Format("01-02 15:04"), decName, f.FileName)
				} else {
					fmt.Printf("- %10d  %s  %s\n", plainSize, f.ModTime().Format("01-02 15:04"), decName)
				}
			}
		} else {
			if f.IsDir() {
				fmt.Printf("%s/\n", decName)
			} else {
				fmt.Println(decName)
			}
		}
	}
}
