package crypt

import (
	"crypto/aes"
	"encoding/base32"
	"errors"
	"strings"

	"github.com/rfjakob/eme"
	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/crypto/scrypt"
)

const (
	FileMagic       = "RCLONE\x00\x00"
	FileMagicSize   = len(FileMagic)
	FileNonceSize   = 24
	FileHeaderSize  = FileMagicSize + FileNonceSize
	BlockHeaderSize = 16 // Poly1305 Tag
	BlockDataSize   = 64 * 1024
	BlockSize       = BlockHeaderSize + BlockDataSize
)

var defaultSalt = []byte{0xA8, 0x0D, 0xF4, 0x3A, 0x8F, 0xBD, 0x03, 0x08, 0xA7, 0xCA, 0xB8, 0x3E, 0x58, 0x1F, 0x86, 0xB1}
var rcloneBase32 = base32.HexEncoding.WithPadding(base32.NoPadding)

type RcloneCipher struct {
	dataKey   [32]byte
	nameKey   [32]byte
	nameTweak [16]byte
}

func NewRcloneCipher(password, salt string) (*RcloneCipher, error) {
	saltBytes := defaultSalt
	if salt != "" {
		saltBytes = []byte(salt)
	}

	key, err := scrypt.Key([]byte(password), saltBytes, 16384, 8, 1, 80)
	if err != nil {
		return nil, err
	}

	c := &RcloneCipher{}
	copy(c.dataKey[:], key[0:32])
	copy(c.nameKey[:], key[32:64])
	copy(c.nameTweak[:], key[64:80])
	return c, nil
}

// DecryptBlock 解密一个 rclone 加密分块
func (c *RcloneCipher) DecryptBlock(ciphertext []byte, blockIndex uint64, fileNonce [24]byte) ([]byte, error) {
	// 1. 计算当前块的 Nonce (FileNonce + blockIndex)
	var nonce [24]byte
	copy(nonce[:], fileNonce[:])
	u := blockIndex
	for i := 0; i < 8 && u > 0; i++ {
		u += uint64(nonce[i])
		nonce[i] = byte(u)
		u >>= 8
	}

	// 2. 解密 (secretbox.Open expects ciphertext with 16-byte MAC at the end)
	// rclone stores [MAC(16B)][Data(N)], but secretbox.Open expects [Data][MAC]? 
	// No, secretbox.Seal/Open in Go expect [MAC][Data] format? Actually let's check.
	// In rclone: secretbox.Open(out, ciphertext, nonce, key)
	plaintext, ok := secretbox.Open(nil, ciphertext, &nonce, &c.dataKey)
	if !ok {
		return nil, errors.New("failed to authenticate decrypted block")
	}

	return plaintext, nil
}

// EncryptSegment 加密单个路径段（如文件名或文件夹名）
func (c *RcloneCipher) EncryptSegment(plaintext string) string {
	if plaintext == "" {
		return ""
	}

	// 1. PKCS7 填充
	plaintextBytes := []byte(plaintext)
	paddingLen := 16 - (len(plaintextBytes) % 16)
	for i := 0; i < paddingLen; i++ {
		plaintextBytes = append(plaintextBytes, byte(paddingLen))
	}

	// 2. EME-AES 加密
	block, _ := aes.NewCipher(c.nameKey[:])
	ciphertext := eme.Transform(block, c.nameTweak[:], plaintextBytes, eme.DirectionEncrypt)

	// 3. Base32 编码
	encoded := rcloneBase32.EncodeToString(ciphertext)
	return strings.ToLower(encoded)
}

// DecryptSegment 解密单个路径段
func (c *RcloneCipher) DecryptSegment(encrypted string) (string, error) {
	if encrypted == "" {
		return "", nil
	}

	rawCiphertext, err := rcloneBase32.DecodeString(strings.ToUpper(encrypted))
	if err != nil {
		return "", err
	}

	block, _ := aes.NewCipher(c.nameKey[:])
	plaintextBytes := eme.Transform(block, c.nameTweak[:], rawCiphertext, eme.DirectionDecrypt)

	// 去除 PKCS7 填充
	if len(plaintextBytes) == 0 {
		return "", nil
	}
	paddingLen := int(plaintextBytes[len(plaintextBytes)-1])
	if paddingLen > 0 && paddingLen <= 16 {
		plaintextBytes = plaintextBytes[:len(plaintextBytes)-paddingLen]
	}

	return string(plaintextBytes), nil
}

// EncryptedSize 根据原始大小计算加密后大小
func (c *RcloneCipher) EncryptedSize(size int64) int64 {
	blocks := size / BlockDataSize
	residue := size % BlockDataSize
	encSize := int64(FileHeaderSize) + blocks*(BlockHeaderSize+BlockDataSize)
	if residue != 0 {
		encSize += BlockHeaderSize + residue
	}
	return encSize
}

// DecryptedSize 根据加密后大小计算原始大小
func (c *RcloneCipher) DecryptedSize(size int64) (int64, error) {
	size -= int64(FileHeaderSize)
	if size < 0 {
		return 0, errors.New("file too short")
	}
	blocks := size / BlockSize
	residue := size % BlockSize
	decSize := blocks * BlockDataSize
	if residue != 0 {
		residue -= BlockHeaderSize
		if residue <= 0 {
			return 0, errors.New("bad block header")
		}
		decSize += residue
	}
	return decSize, nil
}
