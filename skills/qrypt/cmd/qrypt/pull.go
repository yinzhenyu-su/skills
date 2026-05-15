package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
)

func runPull(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)
	drv := loadToolDriver(cfg, cipher)

	remotePath := args[0]
	localPath := ""
	if len(args) >= 2 {
		localPath = args[1]
	}

	fullRemotePath := resolveFullPath(cfg.RootPath(), remotePath)
	parentPath := filepath.Dir(fullRemotePath)
	baseName := filepath.Base(fullRemotePath)

	resolver, ok := drv.(pathResolver)
	if !ok {
		fmt.Printf("该驱动不支持路径解析\n")
		os.Exit(1)
	}

	parentFid, err := resolver.ResolvePath(context.Background(), parentPath)
	if err != nil {
		fmt.Printf("无法解析路径: %v\n", err)
		os.Exit(1)
	}

	entries, err := drv.List(context.Background(), parentFid)
	if err != nil {
		fmt.Printf("无法列出目录内容: %v\n", err)
		os.Exit(1)
	}

	encName := cipher.EncryptSegment(baseName)
	var targetEntry drive.Entry
	found := false
	for _, e := range entries {
		if e.Name == encName {
			targetEntry = e
			found = true
			break
		}
	}
	if !found {
		fmt.Printf("文件未找到: %s\n", baseName)
		os.Exit(1)
	}
	if targetEntry.IsDir {
		fmt.Printf("错误: %s 是一个目录\n", baseName)
		os.Exit(1)
	}

	if localPath == "" {
		localPath = baseName
	}

	encSize := targetEntry.Size
	fmt.Printf("下载 %s (%s) → %s\n", remotePath, formatBytes(encSize), localPath)

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

	ctx := context.Background()
	headerSize := int64(crypt.FileHeaderSize)
	rc, err := drv.Read(ctx, targetEntry, 0, headerSize)
	if err != nil {
		fmt.Printf("下载文件头失败: %v\n", err)
		os.Exit(1)
	}
	header := make([]byte, headerSize)
	if _, err := io.ReadFull(rc, header); err != nil {
		rc.Close()
		fmt.Printf("读取文件头失败: %v\n", err)
		os.Exit(1)
	}
	rc.Close()

	var fileNonce [crypt.FileNonceSize]byte
	copy(fileNonce[:], header[crypt.FileMagicSize:])

	encBodySize := encSize - headerSize
	if encBodySize <= 0 {
		return
	}

	written := int64(0)
	blockIndex := uint64(0)
	readOff := headerSize
	for written < bodySize {
		blockEncSize := int64(crypt.BlockSize)
		remaining := bodySize - written
		if remaining < crypt.BlockDataSize {
			blockEncSize = int64(crypt.BlockHeaderSize + remaining)
		}

		rc, err := drv.Read(ctx, targetEntry, readOff, blockEncSize)
		if err != nil {
			fmt.Printf("下载失败: %v\n", err)
			os.Exit(1)
		}
		encBlock := make([]byte, blockEncSize)
		if _, err := io.ReadFull(rc, encBlock); err != nil {
			rc.Close()
			fmt.Printf("读取数据块失败: %v\n", err)
			os.Exit(1)
		}
		rc.Close()

		plaintext, err := cipher.DecryptBlock(encBlock, blockIndex, fileNonce)
		if err != nil {
			fmt.Printf("解密失败: %v\n", err)
			os.Exit(1)
		}
		outFile.Write(plaintext)
		written += int64(len(plaintext))
		readOff += blockEncSize
		blockIndex++
	}
}
