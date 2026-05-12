package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/quark"
)

func runMv(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)
	quarkClient := quark.NewClient(cfg.Quark.Cookie)
	cacheSvc := quark.NewCacheService()
	fileSvc := quark.NewFileService(quarkClient, cacheSvc, cipher)
	manageSvc := quark.NewManageService(quarkClient)

	if err := fileSvc.Auth(); err != nil {
		fmt.Printf("认证失败: %v\n", err)
		os.Exit(1)
	}

	srcPath := args[0]
	dstArg := args[1]
	fullSrcPath := resolveFullPath(cfg.Quark.RootPath, srcPath)

	srcFid, err := fileSvc.ResolvePath(fullSrcPath)
	if err != nil {
		fmt.Printf("无法解析源路径: %v\n", err)
		os.Exit(1)
	}

	moveIntoDir := strings.HasSuffix(dstArg, "/") || strings.HasSuffix(dstArg, "/ ")

	var dstName string
	var dstParentFid string

	if moveIntoDir {
		fullDstDir := resolveFullPath(cfg.Quark.RootPath, dstArg)
		dstParentFid, err = fileSvc.ResolvePath(fullDstDir)
		if err != nil {
			fmt.Printf("无法解析目标目录: %v\n", err)
			os.Exit(1)
		}

		srcParentPath := filepath.Dir(fullSrcPath)
		srcParentFid, err := fileSvc.ResolvePath(srcParentPath)
		if err != nil {
			fmt.Printf("无法解析源目录: %v\n", err)
			os.Exit(1)
		}
		files, err := fileSvc.ListFiles(srcParentFid)
		if err != nil {
			fmt.Printf("无法列出文件: %v\n", err)
			os.Exit(1)
		}
		for _, f := range files {
			if f.Fid == srcFid {
				decName, decErr := cipher.DecryptSegment(f.FileName)
				if decErr == nil {
					dstName = cipher.EncryptSegment(decName)
				} else {
					dstName = f.FileName
				}
				break
			}
		}
		if dstName == "" {
			fmt.Printf("无法确定文件名\n")
			os.Exit(1)
		}
	} else {
		fullDstPath := resolveFullPath(cfg.Quark.RootPath, dstArg)
		dstParentPath := filepath.Dir(fullDstPath)
		dstNameArg := filepath.Base(fullDstPath)

		dstParentFid, err = fileSvc.ResolvePath(dstParentPath)
		if err != nil {
			fmt.Printf("无法解析目标路径: %v\n", err)
			os.Exit(1)
		}

		if dstParentFid != "0" {
			existingFiles, _ := fileSvc.ListFiles(dstParentFid)
			for _, f := range existingFiles {
				if f.Fid == srcFid {
					continue
				}
				decName, decErr := cipher.DecryptSegment(f.FileName)
				if decErr == nil && decName == dstNameArg && f.IsDir() {
					dstParentFid = f.Fid
					dstName = ""
					moveIntoDir = true
					break
				}
			}
		}

		if !moveIntoDir {
			dstName = cipher.EncryptSegment(dstNameArg)
		}
	}

	srcParentPath := filepath.Dir(fullSrcPath)
	srcParentFid, err := fileSvc.ResolvePath(srcParentPath)
	if err != nil {
		fmt.Printf("无法解析源目录: %v\n", err)
		os.Exit(1)
	}

	if srcParentFid == dstParentFid && (moveIntoDir || dstName == "") {
		fmt.Println("源和目标相同")
		os.Exit(1)
	}

	if srcParentFid == dstParentFid && !moveIntoDir {
		if err := manageSvc.Rename(srcFid, dstName); err != nil {
			fmt.Printf("重命名失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("已重命名: %s → %s\n", srcPath, dstArg)
	} else {
		if err := manageSvc.Move([]string{srcFid}, dstParentFid, srcParentFid); err != nil {
			fmt.Printf("移动失败: %v\n", err)
			os.Exit(1)
		}
		if !moveIntoDir && dstName != "" {
			if err := manageSvc.Rename(srcFid, dstName); err != nil {
				fmt.Printf("重命名失败: %v\n", err)
				os.Exit(1)
			}
		}
		if moveIntoDir {
			fmt.Printf("已移动: %s → %s\n", srcPath, dstArg)
		} else {
			fmt.Printf("已移动: %s → %s\n", srcPath, dstArg)
		}
	}
}
