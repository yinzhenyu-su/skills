package cipher

import (
	"bytes"
	"strings"
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

func TestRcloneCipher_Obfuscate_EdgeCases(t *testing.T) {
	c, _ := NewRcloneCipher("password", "", "base32", "obfuscate")

	t.Run("empty string", func(t *testing.T) {
		enc := c.EncryptSegment("")
		if enc != "" {
			t.Errorf("expected empty, got %q", enc)
		}
		dec, err := c.DecryptSegment("")
		if err != nil || dec != "" {
			t.Errorf("expected empty, got %q err=%v", dec, err)
		}
	})

	t.Run("single character", func(t *testing.T) {
		name := "a"
		enc := c.EncryptSegment(name)
		dec, err := c.DecryptSegment(enc)
		if err != nil || dec != name {
			t.Errorf("single char: %q -> %q err=%v", name, dec, err)
		}
	})

	t.Run("special characters", func(t *testing.T) {
		names := []string{
			"file with spaces.txt",
			"file_with_underscores.js",
			"file.with.dots",
			"hello!world",
			"test!!double",
			"a!!b!!c",
		}
		for _, name := range names {
			enc := c.EncryptSegment(name)
			dec, err := c.DecryptSegment(enc)
			if err != nil || dec != name {
				t.Errorf("special chars: %q -> %q err=%v", name, dec, err)
			}
		}
	})

	t.Run("numbers only", func(t *testing.T) {
		name := "12345"
		enc := c.EncryptSegment(name)
		dec, err := c.DecryptSegment(enc)
		if err != nil || dec != name {
			t.Errorf("numbers: %q -> %q err=%v", name, dec, err)
		}
	})

	t.Run("uppercase and lowercase", func(t *testing.T) {
		names := []string{
			"README.md",
			"Index.HTML",
			"Makefile",
			".gitignore",
		}
		for _, name := range names {
			enc := c.EncryptSegment(name)
			dec, err := c.DecryptSegment(enc)
			if err != nil || dec != name {
				t.Errorf("case: %q -> %q err=%v", name, dec, err)
			}
		}
	})

	t.Run("very long name", func(t *testing.T) {
		name := strings.Repeat("文件名", 50)
		enc := c.EncryptSegment(name)
		// obfuscate 应保持长度基本不变
		if len(enc) > len(name)+10 {
			t.Errorf("too long: %d vs %d", len(enc), len(name))
		}
		dec, err := c.DecryptSegment(enc)
		if err != nil || dec != name {
			t.Errorf("long name: decryption failed err=%v", err)
		}
	})

	t.Run("conflict suffix stripping", func(t *testing.T) {
		cObf, _ := NewRcloneCipher("password", "", "base32", "obfuscate")
		name := "test.txt"
		enc := cObf.EncryptSegment(name)
		// 模拟网盘追加冲突后缀
		withConflict := enc + " (1)"
		dec, err := cObf.DecryptSegment(withConflict)
		if err != nil || dec != name {
			t.Errorf("conflict suffix: %q from %q err=%v", dec, withConflict, err)
		}
	})
}

func TestRcloneCipher_Obfuscate_Determinism(t *testing.T) {
	c1, _ := NewRcloneCipher("password", "", "base32", "obfuscate")
	c2, _ := NewRcloneCipher("password", "", "base32", "obfuscate")

	names := []string{"test.txt", "电影.mp4", "a"}
	for _, name := range names {
		enc1 := c1.EncryptSegment(name)
		enc2 := c2.EncryptSegment(name)
		if enc1 != enc2 {
			t.Errorf("determinism failed for %q: %q vs %q", name, enc1, enc2)
		}
	}
}

func TestRcloneCipher_Obfuscate_KeySensitivity(t *testing.T) {
	c1, _ := NewRcloneCipher("password1", "", "base32", "obfuscate")
	c2, _ := NewRcloneCipher("password2", "", "base32", "obfuscate")

	names := []string{"test.txt", "电影.mp4"}
	for _, name := range names {
		enc1 := c1.EncryptSegment(name)
		enc2 := c2.EncryptSegment(name)
		if enc1 == enc2 {
			t.Errorf("different keys should produce different output for %q", name)
		}
	}
}

func TestRcloneCipher_New_OptDefaults(t *testing.T) {
	// 无 opts → 默认 base32 + standard
	c, err := NewRcloneCipher("p", "")
	if err != nil {
		t.Fatal(err)
	}
	if c.filenameEncoding != "base32" {
		t.Errorf("expected base32, got %s", c.filenameEncoding)
	}
	if c.filenameEncryption != "standard" {
		t.Errorf("expected standard, got %s", c.filenameEncryption)
	}

	// 只传 encoding
	c2, _ := NewRcloneCipher("p", "", "base64")
	if c2.filenameEncoding != "base64" || c2.filenameEncryption != "standard" {
		t.Errorf("unexpected defaults: enc=%s mode=%s", c2.filenameEncoding, c2.filenameEncryption)
	}

	// 传 encoding + encryption
	c3, _ := NewRcloneCipher("p", "", "base64", "obfuscate")
	if c3.filenameEncoding != "base64" || c3.filenameEncryption != "obfuscate" {
		t.Errorf("unexpected: enc=%s mode=%s", c3.filenameEncoding, c3.filenameEncryption)
	}

	// 空 string opt → 默认
	c4, _ := NewRcloneCipher("p", "", "", "")
	if c4.filenameEncoding != "base32" || c4.filenameEncryption != "standard" {
		t.Errorf("empty opts should become defaults: enc=%s mode=%s", c4.filenameEncoding, c4.filenameEncryption)
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
