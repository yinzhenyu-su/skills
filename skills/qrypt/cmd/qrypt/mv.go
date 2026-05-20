package main

import (
	"bufio"
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
	path := args[0]
	mountName := resolveMount(cmd, &path)
	args[0] = path
	drv := loadToolDriverForMount(cfg, cipher, mountName)

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

	interactive, _ := cmd.Flags().GetBool("interactive")
	noClobber, _ := cmd.Flags().GetBool("no-clobber")

	srcPath := args[0]
	dstArg := args[1]
	fullSrcPath := resolveFullPath(cfg.RootPath(), srcPath)

	srcFid, err := resolver.ResolvePath(context.Background(), fullSrcPath)
	if err != nil {
		fmt.Printf("无法解析源路径: %v\n", err)
		os.Exit(1)
	}

	moveIntoDir := strings.HasSuffix(dstArg, "/")

	var dstName string
	var dstParentFid string

	// Helper function to resolve the target name from the source FID
	resolveTargetNameFromSource := func() string {
		srcParentPath := filepath.Dir(fullSrcPath)
		srcParentFid, err := resolver.ResolvePath(context.Background(), srcParentPath)
		if err != nil {
			return ""
		}
		entries, err := drv.List(context.Background(), srcParentFid)
		if err != nil {
			return ""
		}
		for _, e := range entries {
			if e.ID == srcFid {
				decName, decErr := cipher.DecryptSegment(e.Name)
				if decErr == nil {
					return cipher.EncryptSegment(decName)
				}
				return e.Name
			}
		}
		return ""
	}

	if moveIntoDir {
		fullDstDir := resolveFullPath(cfg.RootPath(), dstArg)
		dstParentFid, err = resolver.ResolvePath(context.Background(), fullDstDir)
		if err != nil {
			fmt.Printf("无法解析目标目录: %v\n", err)
			os.Exit(1)
		}
		dstName = resolveTargetNameFromSource()
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
					moveIntoDir = true
					break
				}
			}
		}

		if !moveIntoDir {
			dstName = cipher.EncryptSegment(dstNameArg)
		} else {
			dstName = resolveTargetNameFromSource()
			if dstName == "" {
				fmt.Printf("无法确定文件名\n")
				os.Exit(1)
			}
		}
	}

	ctx := context.Background()

	// Check collision
	existingEntries, err := drv.List(ctx, dstParentFid)
	if err == nil {
		for _, e := range existingEntries {
			if e.Name == dstName && e.ID != srcFid {
				if noClobber {
					fmt.Printf("跳过: 目标已存在 (-n/--no-clobber)\n")
					return
				}
				if interactive {
					fmt.Printf("覆盖目标 %s? (y/N): ", dstArg)
					reader := bufio.NewReader(os.Stdin)
					response, _ := reader.ReadString('\n')
					response = strings.TrimSpace(strings.ToLower(response))
					if response != "y" && response != "yes" {
						fmt.Printf("已取消移动: %s\n", srcPath)
						return
					}
				}
				// Remove existing to simulate overwrite and prevent duplicates
				_ = w.Remove(ctx, e)
				break
			}
		}
	}

	srcParentPath := filepath.Dir(fullSrcPath)
	srcParentFid, err := resolver.ResolvePath(context.Background(), srcParentPath)
	if err != nil {
		fmt.Printf("无法解析源目录: %v\n", err)
		os.Exit(1)
	}

	needsMove := srcParentFid != dstParentFid
	if needsMove {
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

	fmt.Printf("已移动: %s → %s\n", srcPath, dstArg)
}
