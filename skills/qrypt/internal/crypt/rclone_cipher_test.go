package crypt

import (
	"bytes"
	"testing"

	"golang.org/x/crypto/nacl/secretbox"
)

func TestRcloneCipher_KeyDerivation(t *testing.T) {
	// 验证密钥派生是否一致
	password := "testpassword"
	salt := "testsalt"
	c, err := NewRcloneCipher(password, salt)
	if err != nil {
		t.Fatalf("Failed to create cipher: %v", err)
	}

	// 验证 Key 是否被正确分发（32+32+16 = 80字节）
	if len(c.dataKey) != 32 || len(c.nameKey) != 32 || len(c.nameTweak) != 16 {
		t.Errorf("Internal keys have incorrect length")
	}
}

func TestRcloneCipher_FilenameEncryption(t *testing.T) {
	password := "password"
	salt := "" // 默认盐值
	c, _ := NewRcloneCipher(password, salt)

	testNames := []string{
		"README.md",
		"电影.mp4",
		"a",
		"very_long_filename_that_exceeds_multiple_blocks_of_eme_encryption.txt",
	}

	for _, name := range testNames {
		encrypted := c.EncryptSegment(name)
		decrypted, err := c.DecryptSegment(encrypted)
		if err != nil {
			t.Errorf("Decryption failed for %s: %v", name, err)
			continue
		}
		if decrypted != name {
			t.Errorf("Name mismatch! Original: %s, Decrypted: %s", name, decrypted)
		}
	}
}

func TestRcloneCipher_BlockDecryption(t *testing.T) {
	c, _ := NewRcloneCipher("password", "")
	
	// 模拟一个随机 Nonce
	var fileNonce [24]byte
	copy(fileNonce[:], []byte("123456789012345678901234"))

	plaintext := make([]byte, BlockDataSize)
	for i := range plaintext {
		plaintext[i] = byte(i % 256)
	}

	// 模拟 rclone 加密过程 (简单直接调用 secretbox)
	// 注意：rclone 的分块加密是针对每一个 64KB 块的
	// 这里我们需要模拟第 0 个块的加密
	
	// 1. 计算块 Nonce
	nonce := fileNonce
	// (无需累加，因为是第 0 块)

	// 2. 加密
	fromSecretbox := secretbox.Seal(nil, plaintext, &nonce, &c.dataKey)

	// 3. 测试解密
	gotPlaintext, err := c.DecryptBlock(fromSecretbox, 0, fileNonce)
	if err != nil {
		t.Fatalf("DecryptBlock failed: %v", err)
	}

	if !bytes.Equal(plaintext, gotPlaintext) {
		t.Errorf("Plaintext mismatch after block decryption")
	}
}

func TestSizeMapping(t *testing.T) {
	c, _ := NewRcloneCipher("p", "")
	
	testSizes := []int64{0, 1, 100, BlockDataSize, BlockDataSize + 1, 10 * 1024 * 1024}
	for _, size := range testSizes {
		enc := c.EncryptedSize(size)
		dec, err := c.DecryptedSize(enc)
		if err != nil {
			t.Errorf("DecryptedSize failed for size %d: %v", size, err)
			continue
		}
		if dec != size {
			t.Errorf("Size mismatch! Original: %d, Decrypted: %d (Encrypted was %d)", size, dec, enc)
		}
	}
}
