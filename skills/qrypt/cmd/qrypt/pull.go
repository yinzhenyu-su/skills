package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/quark"
)

func runPull(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)
	quarkClient := quark.NewClient(cfg.Quark.Cookie)
	cacheSvc := quark.NewCacheService()
	fileSvc := quark.NewFileService(quarkClient, cacheSvc, cipher)

	if err := fileSvc.Auth(); err != nil {
		fmt.Printf("认证失败: %v\n", err)
		os.Exit(1)
	}

	remotePath := args[0]
	localPath := ""
	if len(args) >= 2 {
		localPath = args[1]
	}

	fullRemotePath := resolveFullPath(cfg.Quark.RootPath, remotePath)
	parentPath := filepath.Dir(fullRemotePath)
	baseName := filepath.Base(fullRemotePath)

	parentFid, err := fileSvc.ResolvePath(parentPath)
	if err != nil {
		fmt.Printf("无法解析路径: %v\n", err)
		os.Exit(1)
	}

	files, err := fileSvc.ListFiles(parentFid)
	if err != nil {
		fmt.Printf("无法列出目录内容: %v\n", err)
		os.Exit(1)
	}

	encName := cipher.EncryptSegment(baseName)
	var targetFile *quark.File
	for i := range files {
		if files[i].FileName == encName {
			targetFile = &files[i]
			break
		}
	}
	if targetFile == nil {
		fmt.Printf("文件未找到: %s\n", baseName)
		os.Exit(1)
	}
	if targetFile.IsDir() {
		fmt.Printf("错误: %s 是一个目录\n", baseName)
		os.Exit(1)
	}

	if localPath == "" {
		localPath = baseName
	}

	fmt.Printf("下载 %s (%s) → %s\n", remotePath, formatBytes(targetFile.Int64Size()), localPath)

	encSize := targetFile.Int64Size()
	fid := targetFile.Fid

	url, err := fileSvc.GetDownloadURL(fid)
	if err != nil {
		fmt.Printf("获取下载链接失败: %v\n", err)
		os.Exit(1)
	}

	outFile, err := os.Create(localPath)
	if err != nil {
		fmt.Printf("创建本地文件失败: %v\n", err)
		os.Exit(1)
	}
	defer outFile.Close()

	bodySize, err := cipher.DecryptedSize(encSize)
	if err != nil {
		fmt.Printf("文件大小异常: %v\n", err)
		os.Exit(1)
	}

	rc, err := quarkClient.DownloadChunk(url, 0, int64(crypt.FileHeaderSize-1))
	if err != nil {
		fmt.Printf("下载文件头失败: %v\n", err)
		os.Exit(1)
	}
	header := make([]byte, crypt.FileHeaderSize)
	if _, err := io.ReadFull(rc, header); err != nil {
		rc.Close()
		fmt.Printf("读取文件头失败: %v\n", err)
		os.Exit(1)
	}
	rc.Close()

	var fileNonce [crypt.FileNonceSize]byte
	copy(fileNonce[:], header[crypt.FileMagicSize:])

	encBodySize := encSize - int64(crypt.FileHeaderSize)
	if encBodySize <= 0 {
		return
	}

	written := int64(0)
	blockIndex := uint64(0)
	for offset := int64(crypt.FileHeaderSize); offset < encSize; offset += int64(crypt.BlockSize) {
		end := offset + int64(crypt.BlockSize) - 1
		if end >= encSize {
			end = encSize - 1
		}

		rc, err := quarkClient.DownloadChunk(url, offset, end)
		if err != nil {
			fmt.Printf("下载数据块失败 (offset=%d): %v\n", offset, err)
			os.Exit(1)
		}

		encBlock := make([]byte, end-offset+1)
		if _, err := io.ReadFull(rc, encBlock); err != nil {
			rc.Close()
			fmt.Printf("读取数据块失败 (offset=%d): %v\n", offset, err)
			os.Exit(1)
		}
		rc.Close()

		plain, err := cipher.DecryptBlock(encBlock, blockIndex, fileNonce)
		if err != nil {
			fmt.Printf("解密数据块失败 (block=%d): %v\n", blockIndex, err)
			os.Exit(1)
		}

		remaining := bodySize - written
		if int64(len(plain)) > remaining {
			plain = plain[:remaining]
		}

		if _, err := outFile.Write(plain); err != nil {
			fmt.Printf("写入本地文件失败: %v\n", err)
			os.Exit(1)
		}
		written += int64(len(plain))
		blockIndex++
	}

	fmt.Printf("完成: %s (%s)\n", localPath, formatBytes(written))
}
