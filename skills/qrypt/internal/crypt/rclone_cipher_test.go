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

	for _, enc := range []string{"base32", "base64"} {
		c, _ := NewRcloneCipher(password, salt, enc)

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
				t.Errorf("[%s] Decryption failed for %s: %v", enc, name, err)
				continue
			}
			if decrypted != name {
				t.Errorf("[%s] Name mismatch! Original: %s, Decrypted: %s", enc, name, decrypted)
			}
		}
	}
}

func TestRcloneCipher_ObfuscateMode(t *testing.T) {
	password := "password"
	salt := ""
	c, _ := NewRcloneCipher(password, salt, "base32", "obfuscate")

	testNames := []string{
		"README.md",
		"电影.mp4",
		"a",
		"hello world",
		"测试中文文件名",
		"very_long_filename_that_exceeds_multiple_blocks.txt",
	}

	for _, name := range testNames {
		encrypted := c.EncryptSegment(name)
		// obfuscate 输出应接近输入长度（仅多数字前缀 + "."）
		if len(encrypted) > len(name)+10 {
			t.Errorf("[obfuscate] output too long for %s: %d vs %d", name, len(encrypted), len(name))
		}
		decrypted, err := c.DecryptSegment(encrypted)
		if err != nil {
			t.Errorf("[obfuscate] decryption failed for %s: %v", name, err)
			continue
		}
		if decrypted != name {
			t.Errorf("[obfuscate] name mismatch! Original: %s, Decrypted: %s", name, decrypted)
		}
	}
}

func TestRcloneCipher_OffMode(t *testing.T) {
	password := "password"
	salt := ""
	c, _ := NewRcloneCipher(password, salt, "base32", "off")

	testNames := []string{
		"README.md",
		"电影.mp4",
		"hello world",
	}

	for _, name := range testNames {
		encrypted := c.EncryptSegment(name)
		if encrypted != name {
			t.Errorf("[off] encrypt should be no-op, got %s", encrypted)
		}
		decrypted, err := c.DecryptSegment(encrypted)
		if err != nil {
			t.Errorf("[off] decrypt failed: %v", err)
		}
		if decrypted != name {
			t.Errorf("[off] name mismatch: %s vs %s", name, decrypted)
		}
	}
}

func TestRcloneCipher_CrossEncodingDecrypt(t *testing.T) {
	password := "password"
	salt := ""
	testNames := []string{
		"README.md",
		"电影.mp4",
		"a",
	}

	// 用 base64 加密，用 base32 配置解密（模拟编码迁移场景）
	c64, _ := NewRcloneCipher(password, salt, "base64")
	c32, _ := NewRcloneCipher(password, salt, "base32")

	for _, name := range testNames {
		encrypted := c64.EncryptSegment(name)
		decrypted, err := c32.DecryptSegment(encrypted)
		if err != nil {
			t.Errorf("[base32←base64] Decryption failed for %s: %v", name, err)
			continue
		}
		if decrypted != name {
			t.Errorf("[base32←base64] Name mismatch! Original: %s, Decrypted: %s", name, decrypted)
		}
	}

	// 反向：用 base32 加密，用 base64 配置解密
	for _, name := range testNames {
		encrypted := c32.EncryptSegment(name)
		decrypted, err := c64.DecryptSegment(encrypted)
		if err != nil {
			t.Errorf("[base64←base32] Decryption failed for %s: %v", name, err)
			continue
		}
		if decrypted != name {
			t.Errorf("[base64←base32] Name mismatch! Original: %s, Decrypted: %s", name, decrypted)
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

func TestRcloneCipher_BlockEncryption(t *testing.T) {
	c, _ := NewRcloneCipher("password", "")

	var fileNonce [24]byte
	copy(fileNonce[:], []byte("123456789012345678901234"))

	plaintext := []byte("hello rclone")

	// 使用我们的 EncryptBlock
	ciphertext, err := c.EncryptBlock(plaintext, 5, fileNonce)
	if err != nil {
		t.Fatalf("EncryptBlock failed: %v", err)
	}

	// 验证解密
	gotPlaintext, err := c.DecryptBlock(ciphertext, 5, fileNonce)
	if err != nil {
		t.Fatalf("DecryptBlock failed: %v", err)
	}

	if !bytes.Equal(plaintext, gotPlaintext) {
		t.Errorf("Plaintext mismatch! Expected: %s, Got: %s", string(plaintext), string(gotPlaintext))
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
