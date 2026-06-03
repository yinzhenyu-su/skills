package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

func runEncrypt(cmd *cobra.Command, args []string) {
	if isFile, _ := cmd.Flags().GetBool("file"); isFile {
		encryptFile(cmd, args[0])
		return
	}
	cp := loadCipherForTool(cmd)
	name := args[0]
	encName := cp.EncryptSegment(name)
	fmt.Printf("明文:  %s\n加密:  %s\n", name, encName)
}

func encryptFile(cmd *cobra.Command, path string) {
	var r io.ReadCloser
	var plainSize int64
	if path == "-" {
		r = io.NopCloser(os.Stdin)
		plainSize = math.MaxInt64
	} else {
		f, err := os.Open(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "打开文件失败: %v\n", err)
			os.Exit(1)
		}
		fi, err := f.Stat()
		if err != nil {
			f.Close()
			fmt.Fprintf(os.Stderr, "获取文件信息失败: %v\n", err)
			os.Exit(1)
		}
		r = f
		plainSize = fi.Size()
	}
	defer r.Close()

	cp := loadCipherForTool(cmd)
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		fmt.Fprintf(os.Stderr, "生成随机数失败: %v\n", err)
		os.Exit(1)
	}

	er := qrypt.NewEncryptingReader(r, cp, nonce, plainSize)
	if _, err := io.Copy(os.Stdout, er); err != nil {
		fmt.Fprintf(os.Stderr, "加密失败: %v\n", err)
		os.Exit(1)
	}
}

func runDecrypt(cmd *cobra.Command, args []string) {
	if isFile, _ := cmd.Flags().GetBool("file"); isFile {
		decryptFile(cmd, args[0])
		return
	}
	cp := loadCipherForTool(cmd)
	encName := args[0]
	plain, err := cp.DecryptSegment(encName)
	if err != nil {
		fmt.Printf("解密失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("加密:  %s\n明文:  %s\n", encName, plain)
}

func decryptFile(cmd *cobra.Command, path string) {
	var r io.ReadCloser
	if path == "-" {
		r = io.NopCloser(os.Stdin)
	} else {
		f, err := os.Open(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "打开文件失败: %v\n", err)
			os.Exit(1)
		}
		r = f
	}
	defer r.Close()

	header := make([]byte, qrypt.FileHeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		fmt.Fprintf(os.Stderr, "读取文件头失败: %v\n", err)
		os.Exit(1)
	}

	if string(header[:qrypt.FileMagicSize]) != qrypt.FileMagic {
		fmt.Fprintf(os.Stderr, "无效的加密文件: 魔数不匹配\n")
		os.Exit(1)
	}

	var nonce [24]byte
	copy(nonce[:], header[qrypt.FileMagicSize:])

	cp := loadCipherForTool(cmd)
	dr := qrypt.NewDecryptingReader(r, cp, nonce)

	if _, err := io.Copy(os.Stdout, dr); err != nil {
		fmt.Fprintf(os.Stderr, "解密失败: %v\n", err)
		os.Exit(1)
	}
}

func runEncSize(cmd *cobra.Command, args []string) {
	cp := loadCipherForTool(cmd)
	size, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		fmt.Printf("无效大小: %s\n", args[0])
		os.Exit(1)
	}
	encSize := cp.EncryptedSize(size)
	fmt.Printf("明文大小:  %d\n加密大小:  %d\n", size, encSize)
}

// loadCipherForTool loads the encryption cipher from config for local tool operations.
func loadCipherForTool(cmd *cobra.Command) *qrypt.RcloneCipher {
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

	cp, err := config.MakeCipher(mountEnc, cfg.Defaults.Encryption, pwd, salt)
	if err != nil {
		fmt.Printf("加密引擎初始化失败: %v\n", err)
		os.Exit(1)
	}
	return cp
}
