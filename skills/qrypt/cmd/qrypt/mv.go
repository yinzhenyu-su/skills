package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
)

func runMv(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)
	drv := loadToolDriver(cfg, cipher)

	w, wOk := drv.(drive.Writer)
	if !wOk {
		fmt.Printf("该驱动不支持移动操作\n")
		os.Exit(1)
	}

	resolver, rOk := drv.(pathResolver)
	if !rOk {
		fmt.Printf("该驱动不支持路径解析\n")
		os.Exit(1)
	}

	srcPath := args[0]
	dstArg := args[1]
	fullSrcPath := resolveFullPath(cfg.RootPath(), srcPath)

	srcFid, err := resolver.ResolvePath(context.Background(), fullSrcPath)
	if err != nil {
		fmt.Printf("无法解析源路径: %v\n", err)
		os.Exit(1)
	}

	moveIntoDir := strings.HasSuffix(dstArg, "/") || strings.HasSuffix(dstArg, "/ ")

	var dstName string
	var dstParentFid string

	if moveIntoDir {
		fullDstDir := resolveFullPath(cfg.RootPath(), dstArg)
		dstParentFid, err = resolver.ResolvePath(context.Background(), fullDstDir)
		if err != nil {
			fmt.Printf("无法解析目标目录: %v\n", err)
			os.Exit(1)
		}

		srcParentPath := filepath.Dir(fullSrcPath)
		srcParentFid, err := resolver.ResolvePath(context.Background(), srcParentPath)
		if err != nil {
			fmt.Printf("无法解析源目录: %v\n", err)
			os.Exit(1)
		}
		entries, err := drv.List(context.Background(), srcParentFid)
		if err != nil {
			fmt.Printf("无法列出文件: %v\n", err)
			os.Exit(1)
		}
		for _, e := range entries {
			if e.ID == srcFid {
				decName, decErr := cipher.DecryptSegment(e.Name)
				if decErr == nil {
					dstName = cipher.EncryptSegment(decName)
				} else {
					dstName = e.Name
				}
				break
			}
		}
		if dstName == "" {
			fmt.Printf("无法确定文件名\n")
			os.Exit(1)
		}
	} else {
		fullDstPath := resolveFullPath(cfg.RootPath(), dstArg)
		dstParentPath := filepath.Dir(fullDstPath)
		dstNameArg := filepath.Base(fullDstPath)

		dstParentFid, err = resolver.ResolvePath(context.Background(), dstParentPath)
		if err != nil {
			fmt.Printf("无法解析目标路径: %v\n", err)
			os.Exit(1)
		}

		if dstParentFid != "0" {
			existingEntries, _ := drv.List(context.Background(), dstParentFid)
			for _, e := range existingEntries {
				if e.ID == srcFid {
					continue
				}
				decName, decErr := cipher.DecryptSegment(e.Name)
				if decErr == nil && decName == dstNameArg && e.IsDir {
					dstParentFid = e.ID
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

	ctx := context.Background()

	srcParentPath := filepath.Dir(fullSrcPath)
	srcParentFid, err := resolver.ResolvePath(context.Background(), srcParentPath)
	if err != nil {
		fmt.Printf("无法解析源目录: %v\n", err)
		os.Exit(1)
	}

	needsMove := srcParentFid != dstParentFid
	if needsMove || moveIntoDir {
		moveEntry := drive.Entry{ID: srcFid}
		if err := w.Move(ctx, moveEntry, dstParentFid); err != nil {
			fmt.Printf("移动失败: %v\n", err)
			os.Exit(1)
		}
	}

	srcName := filepath.Base(fullSrcPath)
	srcEncName := cipher.EncryptSegment(srcName)

	if dstName != srcEncName {
		renameEntry := drive.Entry{ID: srcFid}
		if err := w.Rename(ctx, renameEntry, dstName); err != nil {
			fmt.Printf("重命名失败: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("已重命名: %s → %s\n", srcPath, dstArg)
}
