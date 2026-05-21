package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
)

func runEncrypt(cmd *cobra.Command, args []string) {
	cipher := loadCipherForTool(cmd)
	name := args[0]
	encName := cipher.EncryptSegment(name)
	fmt.Printf("明文:  %s\n加密:  %s\n", name, encName)
}

func runDecrypt(cmd *cobra.Command, args []string) {
	cipher := loadCipherForTool(cmd)
	encName := args[0]
	plain, err := cipher.DecryptSegment(encName)
	if err != nil {
		fmt.Printf("解密失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("加密:  %s\n明文:  %s\n", encName, plain)
}

func runEncSize(cmd *cobra.Command, args []string) {
	cipher := loadCipherForTool(cmd)
	size, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		fmt.Printf("无效大小: %s\n", args[0])
		os.Exit(1)
	}
	encSize := cipher.EncryptedSize(size)
	fmt.Printf("明文大小:  %d\n加密大小:  %d\n", size, encSize)
}

// loadCipherForTool loads the encryption cipher from config for local tool operations.
func loadCipherForTool(cmd *cobra.Command) *crypt.RcloneCipher {
	configPath, _ := cmd.Flags().GetString("config")
	_, cfg, _, _ := config.LoadConfigAuto(configPath)
	if cfg == nil {
		fmt.Println("未找到配置文件，请使用 --config 指定")
		os.Exit(1)
	}

	pwd, _ := cmd.Flags().GetString("password")
	salt, _ := cmd.Flags().GetString("salt")

	var mountEnc config.EncryptionConfig
	if m := config.FindDefaultMount(cfg); m != nil {
		rc := cfg.MergeInstanceConfig(*m)
		mountEnc = rc.Encryption
	}

	cipher, err := config.MakeCipher(mountEnc, cfg.Defaults.Encryption, pwd, salt)
	if err != nil {
		fmt.Printf("加密引擎初始化失败: %v\n", err)
		os.Exit(1)
	}
	return cipher
}
