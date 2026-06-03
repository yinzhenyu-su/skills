package crypt

import (
	"bytes"
	"crypto/aes"
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

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
var rcloneBase64 = base64.URLEncoding.WithPadding(base64.NoPadding)

// conflictSuffixRe 匹配 Quark Drive 等网盘追加的 (N) 冲突后缀
var conflictSuffixRe = regexp.MustCompile(`^(.*?)\s*\(\d+\)$`)

const obfuscQuoteRune = '!'

type RcloneCipher struct {
	dataKey            [32]byte
	nameKey            [32]byte
	nameTweak          [16]byte
	filenameEncryption string // "standard", "obfuscate", "off"
	filenameEncoding   string // "base32", "base64" (only for "standard")
}

func NewRcloneCipher(password, salt string, opts ...string) (*RcloneCipher, error) {
	saltBytes := defaultSalt
	if salt != "" {
		saltBytes = []byte(salt)
	}

	key, err := scrypt.Key([]byte(password), saltBytes, 16384, 8, 1, 80)
	if err != nil {
		return nil, err
	}

	encoding := "base32"
	encryption := "standard"
	for i, opt := range opts {
		switch i {
		case 0:
			if opt != "" {
				encoding = opt
			}
		case 1:
			if opt != "" {
				encryption = opt
			}
		}
	}

	c := &RcloneCipher{}
	copy(c.dataKey[:], key[0:32])
	copy(c.nameKey[:], key[32:64])
	copy(c.nameTweak[:], key[64:80])
	c.filenameEncoding = encoding
	c.filenameEncryption = encryption
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

// EncryptBlock 加密一个 rclone 分块
func (c *RcloneCipher) EncryptBlock(plaintext []byte, blockIndex uint64, fileNonce [24]byte) ([]byte, error) {
	// 1. 计算当前块的 Nonce (FileNonce + blockIndex)
	var nonce [24]byte
	copy(nonce[:], fileNonce[:])
	u := blockIndex
	for i := 0; i < 8 && u > 0; i++ {
		u += uint64(nonce[i])
		nonce[i] = byte(u)
		u >>= 8
	}

	// 2. 加密
	// secretbox.Seal appends the MAC (16B) to the ciphertext.
	// rclone format: [MAC(16B)][Data(N)]
	ciphertext := secretbox.Seal(nil, plaintext, &nonce, &c.dataKey)
	return ciphertext, nil
}

// GenerateRandomNonce 生成一个新的随机 24 字节 Nonce
func (c *RcloneCipher) GenerateRandomNonce() ([24]byte, error) {
	var nonce [24]byte
	_, err := io.ReadFull(rand.Reader, nonce[:])
	return nonce, err
}

// EncryptSegment 加密单个路径段（如文件名或文件夹名）
func (c *RcloneCipher) EncryptSegment(plaintext string) string {
	if plaintext == "" {
		return ""
	}

	switch c.filenameEncryption {
	case "off":
		return plaintext
	case "obfuscate":
		return c.obfuscateSegment(plaintext)
	default: // "standard"
		return c.encryptSegmentStandard(plaintext)
	}
}

// encryptSegmentStandard EME-AES + base32/base64 加密
func (c *RcloneCipher) encryptSegmentStandard(plaintext string) string {
	// 1. PKCS7 填充
	plaintextBytes := []byte(plaintext)
	paddingLen := 16 - (len(plaintextBytes) % 16)
	for i := 0; i < paddingLen; i++ {
		plaintextBytes = append(plaintextBytes, byte(paddingLen))
	}

	// 2. EME-AES 加密
	block, _ := aes.NewCipher(c.nameKey[:])
	ciphertext := eme.Transform(block, c.nameTweak[:], plaintextBytes, eme.DirectionEncrypt)

	// 3. 按配置编码
	switch c.filenameEncoding {
	case "base64":
		return rcloneBase64.EncodeToString(ciphertext)
	default:
		return strings.ToLower(rcloneBase32.EncodeToString(ciphertext))
	}
}

// DecryptSegment 解密单个路径段，自动处理 (N) 冲突后缀
func (c *RcloneCipher) DecryptSegment(encrypted string) (string, error) {
	if encrypted == "" {
		return "", nil
	}

	switch c.filenameEncryption {
	case "off":
		return encrypted, nil
	case "obfuscate":
		// obfuscate 模式下先剥离 (N) 冲突后缀，
		// 否则后缀字符会被当作 obfuscate 内容误解码
		cleaned := stripConflictSuffix(encrypted)
		return c.deobfuscateSegment(cleaned)
	default:
		plain, err := c.decryptSegmentStandard(encrypted)
		if err == nil {
			return plain, nil
		}
		// standard 模式：冲突后缀导致解码失败 → 剥离后重试
		cleaned := stripConflictSuffix(encrypted)
		if cleaned != encrypted {
			return c.decryptSegmentStandard(cleaned)
		}
		return "", err
	}
}

// decryptSegmentStandard EME-AES 解码 + 双编码 fallback
func (c *RcloneCipher) decryptSegmentStandard(encrypted string) (string, error) {
	// 优先使用配置的编码
	for _, enc := range []string{c.filenameEncoding, otherEncoding(c.filenameEncoding)} {
		plain, err := c.decodeAndDecrypt(encrypted, enc)
		if err == nil {
			return plain, nil
		}
	}

	// 尝试剥离 (N) / (N) 冲突后缀后重试
	cleaned := stripConflictSuffix(encrypted)
	if cleaned != encrypted {
		return c.decryptSegmentStandard(cleaned)
	}

	return "", errors.New("failed to decrypt filename")
}

func otherEncoding(enc string) string {
	if enc == "base64" {
		return "base32"
	}
	return "base64"
}

// decodeAndDecrypt base32/base64 -> EME-AES 解密 -> 去填充
func (c *RcloneCipher) decodeAndDecrypt(encrypted, encoding string) (string, error) {
	if encrypted == "" {
		return "", nil
	}

	var rawCiphertext []byte
	var err error
	switch encoding {
	case "base64":
		rawCiphertext, err = rcloneBase64.DecodeString(encrypted)
	default:
		rawCiphertext, err = rcloneBase32.DecodeString(strings.ToUpper(encrypted))
	}
	if err != nil {
		return "", err
	}

	if len(rawCiphertext) == 0 {
		return "", errors.New("empty ciphertext")
	}
	if len(rawCiphertext)%16 != 0 {
		return "", errors.New("ciphertext length is not a multiple of 16")
	}

	block, _ := aes.NewCipher(c.nameKey[:])
	plaintextBytes := eme.Transform(block, c.nameTweak[:], rawCiphertext, eme.DirectionDecrypt)

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
	if size <= 0 {
		return 0, nil
	}
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

// ──────────────────────────────────────────────
// obfuscate 模式（rclone 兼容，长度不变）
// ──────────────────────────────────────────────

func (c *RcloneCipher) obfuscateSegment(plaintext string) string {
	if plaintext == "" {
		return ""
	}
	if !utf8.ValidString(plaintext) {
		return "!." + plaintext
	}

	var dir int
	for _, runeValue := range plaintext {
		dir += int(runeValue)
	}
	dir %= 256

	var result bytes.Buffer
	result.WriteString(strconv.Itoa(dir))
	result.WriteByte('.')
	for i := range len(c.nameKey) {
		dir += int(c.nameKey[i])
	}

	for _, runeValue := range plaintext {
		switch {
		case runeValue == obfuscQuoteRune:
			result.WriteRune(obfuscQuoteRune)
			result.WriteRune(obfuscQuoteRune)

		case runeValue >= '0' && runeValue <= '9':
			thisdir := (dir % 9) + 1
			newRune := '0' + (int(runeValue)-'0'+thisdir)%10
			result.WriteRune(rune(newRune))

		case (runeValue >= 'A' && runeValue <= 'Z') ||
			(runeValue >= 'a' && runeValue <= 'z'):
			thisdir := dir%25 + 1
			pos := int(runeValue - 'A')
			if pos >= 26 {
				pos -= 6
			}
			pos = (pos + thisdir) % 52
			if pos >= 26 {
				pos += 6
			}
			result.WriteRune(rune('A' + pos))

		case runeValue >= 0xA0 && runeValue <= 0xFF:
			thisdir := (dir % 95) + 1
			newRune := 0xA0 + (int(runeValue)-0xA0+thisdir)%96
			result.WriteRune(rune(newRune))

		case runeValue >= 0x100:
			thisdir := (dir % 127) + 1
			base := int(runeValue - runeValue%256)
			newRune := rune(base + (int(runeValue)-base+thisdir)%256)
			if !utf8.ValidRune(newRune) {
				result.WriteRune(obfuscQuoteRune)
				result.WriteRune(runeValue)
			} else {
				result.WriteRune(newRune)
			}

		default:
			result.WriteRune(runeValue)
		}
	}
	return result.String()
}

func (c *RcloneCipher) deobfuscateSegment(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	before, after, ok := strings.Cut(ciphertext, ".")
	if !ok {
		return "", errors.New("not an obfuscated file")
	}
	num := before
	if num == "!" {
		return after, nil
	}
	dir, err := strconv.Atoi(num)
	if err != nil {
		return "", errors.New("not an obfuscated file")
	}
	for i := range len(c.nameKey) {
		dir += int(c.nameKey[i])
	}

	var result bytes.Buffer
	inQuote := false
	for _, runeValue := range after {
		if inQuote {
			result.WriteRune(runeValue)
			inQuote = false
			continue
		}
		if runeValue == obfuscQuoteRune {
			inQuote = true
			continue
		}
		switch {
		case runeValue >= '0' && runeValue <= '9':
			thisdir := (dir % 9) + 1
			orig := (int(runeValue) - '0' - thisdir) % 10
			if orig < 0 {
				orig += 10
			}
			result.WriteRune(rune('0' + orig))

		case (runeValue >= 'A' && runeValue <= 'Z') ||
			(runeValue >= 'a' && runeValue <= 'z'):
			thisdir := dir%25 + 1
			pos := int(runeValue - 'A')
			if pos >= 26 {
				pos -= 6
			}
			pos = (pos - thisdir) % 52
			if pos < 0 {
				pos += 52
			}
			if pos >= 26 {
				pos += 6
			}
			result.WriteRune(rune('A' + pos))

		case runeValue >= 0xA0 && runeValue <= 0xFF:
			thisdir := (dir % 95) + 1
			orig := (int(runeValue) - 0xA0 - thisdir) % 96
			if orig < 0 {
				orig += 96
			}
			result.WriteRune(rune(0xA0 + orig))

		case runeValue >= 0x100:
			thisdir := (dir % 127) + 1
			base := int(runeValue - runeValue%256)
			orig := (int(runeValue) - base - thisdir) % 256
			if orig < 0 {
				orig += 256
			}
			result.WriteRune(rune(base + orig))

		default:
			result.WriteRune(runeValue)
		}
	}
	return result.String(), nil
}

// stripConflictSuffix 剥离 (N) /  (N) 等网盘冲突后缀，返回清理后的文件名
func stripConflictSuffix(name string) string {
	matches := conflictSuffixRe.FindStringSubmatch(name)
	if len(matches) == 2 {
		return matches[1]
	}
	return name
}

// HasConflictSuffix 检查文件名是否带有 (N) 冲突后缀
func HasConflictSuffix(name string) bool {
	return conflictSuffixRe.MatchString(name)
}
