package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
	"github.com/yinzhenyu/skills/qrypt/internal/sync"
)

func runPush(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)

	localPath := args[0]
	remotePath := ""
	if len(args) >= 2 {
		remotePath = args[1]
	}
	mountName := resolveMount(cmd, &remotePath)
	drv := loadToolDriverForMount(cfg, cipher, mountName)
	resolver, _ := drv.(pathResolver)

	update, _ := cmd.Flags().GetBool("update")
	transfers, _ := cmd.Flags().GetInt("transfers")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	isStdin := localPath == "-"
	var localInfo os.FileInfo
	var err error

	if !isStdin {
		localInfo, err = os.Stat(localPath)
		if err != nil {
			fmt.Printf("无法读取本地文件/目录信息: %v\n", err)
			os.Exit(1)
		}
	}

	fullRemotePath := resolveFullPath(cfg.RootPath(), remotePath)
	var parentFid string

	if !isStdin && localInfo.IsDir() {
		if remotePath == "" {
			fullRemotePath = resolveFullPath(cfg.RootPath(), filepath.Base(localPath))
		}

		remoteParentPath := filepath.Dir(fullRemotePath)
		remoteDirName := filepath.Base(fullRemotePath)

		parentFid, err = resolver.ResolvePath(context.Background(), remoteParentPath)
		if err != nil {
			fmt.Printf("无法解析目标路径的父目录: %v\n", err)
			os.Exit(1)
		}

		w, hasMkdir := drv.(drive.Writer)
		if hasMkdir && !dryRun {
			entries, _ := drv.List(context.Background(), parentFid)
			found := false
			encDirName := cipher.EncryptSegment(remoteDirName)
			for _, e := range entries {
				if e.IsDir && e.Name == encDirName {
					parentFid = e.ID
					found = true
					break
				}
			}
			if !found {
				newEntry, err := w.Mkdir(context.Background(), parentFid, encDirName)
				if err != nil {
					fmt.Printf("无法创建目标根目录: %v\n", err)
					os.Exit(1)
				}
				parentFid = newEntry.ID
			}
		} else if dryRun {
			fmt.Printf("[Dry Run] Would resolve/create target directory %s\n", remoteDirName)
			parentFid = "mock-" + remoteDirName
		}

		pool := sync.NewWorkerPool(drv, cipher, transfers, dryRun, update)
		pool.Start(context.Background())

		fmt.Printf("开始递归上传目录: %s\n", localPath)
		if err := sync.ScanLocalForUpload(context.Background(), localPath, parentFid, drv, cipher, pool); err != nil {
			fmt.Printf("扫描本地目录失败: %v\n", err)
		}

		pool.Wait()
		fmt.Printf("批量上传完成\n")
		return
	}

	localFileName := filepath.Base(localPath)
	if isStdin {
		localFileName = "stdin"
	}
	var remoteFileName string

	if remotePath == "" {
		remoteFileName = localFileName
	} else {
		remoteFileName = filepath.Base(remotePath)
	}

	if remotePath != "" && (strings.HasSuffix(remotePath, "/") || strings.HasSuffix(remotePath, "/ ")) {
		parentFid, err = resolver.ResolvePath(context.Background(), fullRemotePath)
		if err != nil {
			fmt.Printf("无法解析目标路径: %v\n", err)
			os.Exit(1)
		}
		remoteFileName = localFileName
	} else {
		remoteParentPath := filepath.Dir(fullRemotePath)
		remoteFileName = filepath.Base(fullRemotePath)
		parentFid, err = resolver.ResolvePath(context.Background(), remoteParentPath)
		if err != nil {
			fmt.Printf("无法解析目标路径: %v\n", err)
			os.Exit(1)
		}
	}

	if isStdin {
		tmpFile, err := os.CreateTemp("", "qrypt-stdin-*")
		if err != nil {
			fmt.Printf("无法创建临时文件: %v\n", err)
			os.Exit(1)
		}
		defer os.Remove(tmpFile.Name())

		written, err := io.Copy(tmpFile, os.Stdin)
		if err != nil {
			tmpFile.Close()
			fmt.Printf("读取标准输入失败: %v\n", err)
			os.Exit(1)
		}
		tmpFile.Close()

		uploader := sync.NewUploader(drv, cipher)
		req := sync.Request{
			Name:      remoteFileName,
			ParentFid: parentFid,
			PlainSize: written,
			DataReader: func() (io.ReadCloser, error) {
				return os.Open(tmpFile.Name())
			},
			ProgressFn: func(partNumber int) {
				fmt.Printf("  已上传 %d 个分块\n", partNumber)
			},
		}
		if dryRun {
			fmt.Printf("[Dry Run] Would upload stdin (%s) to %s\n", formatBytes(written), remoteFileName)
			return
		}
		result, err := uploader.Upload(context.Background(), req)
		if err != nil {
			fmt.Printf("上传失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("完成: fid=%s, 加密大小=%s\n", result.Fid, formatBytes(result.EncryptedSize))
		return
	}

	pool := sync.NewWorkerPool(drv, cipher, transfers, dryRun, update)
	pool.Start(context.Background())

	pool.Submit(sync.TransferJob{
		Type:         sync.JobTypeUpload,
		LocalPath:    localPath,
		RemoteName:   remoteFileName,
		RemoteParent: parentFid,
		Size:         localInfo.Size(),
	})

	pool.Wait()
}
