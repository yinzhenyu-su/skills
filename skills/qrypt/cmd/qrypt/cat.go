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

func runCat(cmd *cobra.Command, args []string) {
	cfg, cipher := loadToolCfg(cmd)

	path := args[0]
	mountName := resolveMount(cmd, &path)
	drv := loadToolDriverForMount(cfg, cipher, mountName)
	fullPath := resolveFullPath(cfg.RootPath(), path)
	parentPath := filepath.Dir(fullPath)
	baseName := filepath.Base(fullPath)

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
	var targetEntry *drive.Entry
	for i := range entries {
		if entries[i].Name == encName {
			targetEntry = &entries[i]
			break
		}
	}
	if targetEntry == nil {
		fmt.Printf("文件未找到: %s\n", baseName)
		os.Exit(1)
	}
	if targetEntry.IsDir {
		fmt.Printf("错误: %s 是一个目录\n", baseName)
		os.Exit(1)
	}

	encSize := targetEntry.Size
	headerSize := int64(crypt.FileHeaderSize)
	header, err := drv.Read(context.Background(), *targetEntry, 0, headerSize)
	if err != nil {
		fmt.Printf("读取文件头失败: %v\n", err)
		os.Exit(1)
	}
	headerBytes := make([]byte, headerSize)
	if _, err := io.ReadFull(header, headerBytes); err != nil {
		header.Close()
		fmt.Printf("读取文件头失败: %v\n", err)
		os.Exit(1)
	}
	header.Close()

	if string(headerBytes[:len(crypt.FileMagic)]) != crypt.FileMagic {
		fmt.Fprintf(os.Stderr, "警告: 文件格式不是有效的 rclone 加密文件\n")
	}

	var fileNonce [crypt.FileNonceSize]byte
	copy(fileNonce[:], headerBytes[crypt.FileMagicSize:])

	ctx := context.Background()
	bodySize := encSize - headerSize
	if bodySize <= 0 {
		return
	}

	const blocksPerSegment = 128
	const prefetchDist = 4

	segEncBytes := int64(blocksPerSegment * crypt.BlockSize)
	numSegs := int((bodySize + segEncBytes - 1) / segEncBytes)
	segOff := headerSize

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
			off := segOff + int64(idx)*segEncBytes
			sz := segEncBytes
			if idx == numSegs-1 {
				sz = bodySize - int64(idx)*segEncBytes
			}
			rc, err := drv.Read(ctx, *targetEntry, off, sz)
			if err != nil {
				ch <- segResult{idx: idx, err: fmt.Errorf("segment %d: %w", idx, err)}
				return
			}
			data := make([]byte, sz)
			if _, err := io.ReadFull(rc, data); err != nil {
				rc.Close()
				ch <- segResult{idx: idx, err: fmt.Errorf("segment %d read: %w", idx, err)}
				return
			}
			rc.Close()
			ch <- segResult{idx: idx, data: data}
		}()
		futures = append(futures, segFuture{ch: ch, idx: idx})
	}

	for i := 0; i < prefetchDist && i < numSegs; i++ {
		prefetch(i)
	}

	for next := 0; next < numSegs; next++ {
		result := <-futures[0].ch
		futures = futures[1:]

		if result.err != nil {
			fmt.Fprintf(os.Stderr, "下载错误: %v\n", result.err)
			os.Exit(1)
		}

		nextPrefetch := next + prefetchDist
		if nextPrefetch < numSegs {
			prefetch(nextPrefetch)
		}

		plaintext, err := cipher.DecryptBlock(result.data, uint64(result.idx), fileNonce)
		if err != nil {
			fmt.Fprintf(os.Stderr, "解密失败: %v\n", err)
			os.Exit(1)
		}
		os.Stdout.Write(plaintext)
	}
}
