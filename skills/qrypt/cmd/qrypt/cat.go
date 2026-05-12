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

func runCat(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)
	quarkClient := quark.NewClient(cfg.Quark.Cookie)
	cacheSvc := quark.NewCacheService()
	fileSvc := quark.NewFileService(quarkClient, cacheSvc, cipher)

	if err := fileSvc.Auth(); err != nil {
		fmt.Printf("认证失败: %v\n", err)
		os.Exit(1)
	}

	path := args[0]
	fullPath := resolveFullPath(cfg.Quark.RootPath, path)
	parentPath := filepath.Dir(fullPath)
	baseName := filepath.Base(fullPath)

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

	encSize := targetFile.Int64Size()
	fid := targetFile.Fid
	url, err := fileSvc.GetDownloadURL(fid)
	if err != nil {
		fmt.Printf("获取下载链接失败: %v\n", err)
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

	if string(header[:len(crypt.FileMagic)]) != crypt.FileMagic {
		fmt.Fprintf(os.Stderr, "警告: 文件格式不是有效的 rclone 加密文件\n")
	}

	var fileNonce [crypt.FileNonceSize]byte
	copy(fileNonce[:], header[crypt.FileMagicSize:])

	bodySize := encSize - int64(crypt.FileHeaderSize)
	if bodySize <= 0 {
		return
	}

	const blocksPerSegment = 128
	const prefetchDist = 4

	segEncBytes := int64(blocksPerSegment * crypt.BlockSize)
	numSegs := int((bodySize + segEncBytes - 1) / segEncBytes)

	type segResult struct {
		idx  int
		data []byte
		err  error
	}
	type segFuture struct {
		ch  chan segResult
		idx int
	}

	futures := make([]segFuture, 0, prefetchDist)

	prefetch := func(idx int) {
		ch := make(chan segResult, 1)
		go func() {
			start := int64(crypt.FileHeaderSize) + int64(idx)*segEncBytes
			end := start + segEncBytes - 1
			if end >= encSize {
				end = encSize - 1
			}
			rc, err := quarkClient.DownloadChunk(url, start, end)
			if err != nil {
				ch <- segResult{idx: idx, err: err}
				return
			}
			defer rc.Close()
			data, err := io.ReadAll(rc)
			if err != nil {
				ch <- segResult{idx: idx, err: err}
				return
			}
			ch <- segResult{idx: idx, data: data}
		}()
		futures = append(futures, segFuture{ch: ch, idx: idx})
	}

	for i := 0; i < prefetchDist && i < numSegs; i++ {
		prefetch(i)
	}

	nextPrefetch := prefetchDist
	for segIdx := 0; segIdx < numSegs; segIdx++ {
		if nextPrefetch < numSegs {
			prefetch(nextPrefetch)
			nextPrefetch++
		}

		r := <-futures[0].ch
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "\n下载分段 %d 失败: %v\n", r.idx, r.err)
			os.Exit(1)
		}

		processCatSegment(r.data, cipher, fileNonce, segIdx*blocksPerSegment)
		futures = futures[1:]
	}
}

func processCatSegment(data []byte, cipher *crypt.RcloneCipher, fileNonce [crypt.FileNonceSize]byte, startBlockIdx int) {
	blockIdx := startBlockIdx
	for offset := 0; offset < len(data); {
		blockEnd := offset + crypt.BlockSize
		if blockEnd > len(data) {
			blockEnd = len(data)
		}
		encBlock := data[offset:blockEnd]
		offset = blockEnd

		if len(encBlock) <= crypt.BlockHeaderSize {
			break
		}

		plain, err := cipher.DecryptBlock(encBlock, uint64(blockIdx), fileNonce)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\n解密块 %d 失败: %v\n", blockIdx, err)
			os.Exit(1)
		}

		if _, err := os.Stdout.Write(plain); err != nil {
			os.Exit(0)
		}
		blockIdx++
	}
}
