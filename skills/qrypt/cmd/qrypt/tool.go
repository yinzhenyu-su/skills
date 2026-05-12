package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

func runEncrypt(cmd *cobra.Command, args []string) {
	_, cipher := loadToolCfg(cmd)
	name := args[0]
	encName := cipher.EncryptSegment(name)
	fmt.Printf("明文:  %s\n加密:  %s\n", name, encName)
}

func runDecrypt(cmd *cobra.Command, args []string) {
	_, cipher := loadToolCfg(cmd)
	encName := args[0]
	plain, err := cipher.DecryptSegment(encName)
	if err != nil {
		fmt.Printf("解密失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("加密:  %s\n明文:  %s\n", encName, plain)
}

func runEncSize(cmd *cobra.Command, args []string) {
	_, cipher := loadToolCfg(cmd)
	size, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		fmt.Printf("无效大小: %s\n", args[0])
		os.Exit(1)
	}
	encSize := cipher.EncryptedSize(size)
	fmt.Printf("明文大小:  %d\n加密大小:  %d\n", size, encSize)
}
