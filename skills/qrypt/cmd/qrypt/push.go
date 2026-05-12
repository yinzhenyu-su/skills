package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/quark"
	"github.com/yinzhenyu/skills/qrypt/internal/sync"
)

func runPush(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)
	quarkClient := quark.NewClient(cfg.Quark.Cookie)
	cacheSvc := quark.NewCacheService()
	fileSvc := quark.NewFileService(quarkClient, cacheSvc, cipher)
	manageSvc := quark.NewManageService(quarkClient)
	uploadSvc := quark.NewUploadService(quarkClient)

	if err := fileSvc.Auth(); err != nil {
		fmt.Printf("认证失败: %v\n", err)
		os.Exit(1)
	}

	localPath := args[0]
	remotePath := ""
	if len(args) >= 2 {
		remotePath = args[1]
	}

	localFile, err := os.Open(localPath)
	if err != nil {
		fmt.Printf("无法打开本地文件: %v\n", err)
		os.Exit(1)
	}
	localInfo, err := localFile.Stat()
	if err != nil {
		localFile.Close()
		fmt.Printf("无法读取文件信息: %v\n", err)
		os.Exit(1)
	}
	if localInfo.IsDir() {
		localFile.Close()
		fmt.Printf("错误: %s 是一个目录\n", localPath)
		os.Exit(1)
	}

	isStdin := localPath == "-"
	localFileName := filepath.Base(localPath)
	var remoteFileName string

	if remotePath == "" {
		remoteFileName = localFileName
	} else {
		remoteFileName = filepath.Base(remotePath)
	}

	fullRemotePath := resolveFullPath(cfg.Quark.RootPath, remotePath)
	var parentFid string

	stat, err := os.Stat(remotePath)
	if remotePath != "" && err == nil && stat.IsDir() {
		parentFid, err = fileSvc.ResolvePath(fullRemotePath)
		if err != nil {
			fmt.Printf("无法解析目标路径: %v\n", err)
			os.Exit(1)
		}
		remoteFileName = localFileName
	} else {
		remoteParentPath := filepath.Dir(fullRemotePath)
		remoteFileName = filepath.Base(fullRemotePath)
		parentFid, err = fileSvc.ResolvePath(remoteParentPath)
		if err != nil {
			fmt.Printf("无法解析目标路径: %v\n", err)
			os.Exit(1)
		}
	}

	uploader := sync.NewUploader(fileSvc, manageSvc, uploadSvc, cacheSvc, cipher)

	plainSize := localInfo.Size()
	var dataReader func() (io.ReadCloser, error)

	if isStdin {
		dataReader = func() (io.ReadCloser, error) {
			return io.NopCloser(os.Stdin), nil
		}
	} else {
		dataReader = func() (io.ReadCloser, error) {
			return os.Open(localPath)
		}
	}

	fmt.Printf("上传 %s (%s) → %s\n", localPath, formatBytes(plainSize), remoteFileName)
	localFile.Close()

	req := sync.Request{
		Name:      remoteFileName,
		ParentFid: parentFid,
		PlainSize: plainSize,
		DataReader: func() (io.ReadCloser, error) {
			return dataReader()
		},
		ProgressFn: func(partNumber int) {
			fmt.Printf("  已上传 %d 个分块\n", partNumber)
		},
	}

	result, err := uploader.Upload(req)
	if err != nil {
		fmt.Printf("上传失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("完成: fid=%s, 加密大小=%s\n", result.Fid, formatBytes(result.EncryptedSize))
}
