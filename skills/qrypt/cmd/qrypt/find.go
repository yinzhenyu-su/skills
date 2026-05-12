package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/quark"
)

func runFind(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)
	quarkClient := quark.NewClient(cfg.Quark.Cookie)
	cacheSvc := quark.NewCacheService()
	fileSvc := quark.NewFileService(quarkClient, cacheSvc, cipher)

	if err := fileSvc.Auth(); err != nil {
		fmt.Printf("认证失败: %v\n", err)
		os.Exit(1)
	}

	rootPath := "/"
	pattern := ""
	if len(args) >= 1 {
		rootPath = args[0]
	}
	if len(args) >= 2 {
		pattern = args[1]
	} else if len(args) == 1 {
		if !strings.Contains(rootPath, "/") && rootPath != "." {
			pattern = rootPath
			rootPath = "/"
		}
	}

	fullRootPath := resolveFullPath(cfg.Quark.RootPath, rootPath)
	rootFid, err := fileSvc.ResolvePath(fullRootPath)
	if err != nil {
		fmt.Printf("无法解析路径: %v\n", err)
		os.Exit(1)
	}

	matched := 0
	patternLower := strings.ToLower(pattern)
	err = findRecursive(fileSvc, cipher, rootFid, fullRootPath, patternLower, &matched)
	if err != nil {
		fmt.Fprintf(os.Stderr, "搜索出错: %v\n", err)
		os.Exit(1)
	}

	if matched == 0 {
		fmt.Println("未找到匹配的文件")
	}
}

type fileLister interface {
	ListFiles(parentFid string) ([]quark.File, error)
}

func findRecursive(fileSvc fileLister, cipher interface {
	DecryptSegment(string) (string, error)
	EncryptSegment(string) string
}, parentFid, currentPath, patternLower string, matched *int) error {
	files, err := fileSvc.ListFiles(parentFid)
	if err != nil {
		return err
	}

	for _, f := range files {
		decName, decErr := cipher.DecryptSegment(f.FileName)
		if decErr != nil {
			decName = f.FileName
		}

		displayPath := filepath.Join(currentPath, decName)

		if patternLower == "" || strings.Contains(strings.ToLower(decName), patternLower) {
			if f.IsDir() {
				fmt.Printf("%s/\n", displayPath)
			} else {
				fmt.Println(displayPath)
			}
			*matched++
		}

		if f.IsDir() {
			if err := findRecursive(fileSvc, cipher, f.Fid, displayPath, patternLower, matched); err != nil {
				continue
			}
		}
	}
	return nil
}
