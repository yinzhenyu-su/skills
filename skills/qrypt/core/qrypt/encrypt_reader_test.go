package qrypt

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/yinzhenyu/skills/qrypt/cipher"
)

func TestNewEncryptingReader(t *testing.T) {
	c, _ := cipher.NewRcloneCipher("password", "salt")
	var nonce [24]byte
	r := NewEncryptingReader(strings.NewReader("hello"), c, nonce, 5)
	if r == nil {
		t.Fatal("expected non-nil reader")
	}
}

func TestEncryptingReader_Read_Empty(t *testing.T) {
	c, _ := cipher.NewRcloneCipher("password", "salt")
	var nonce [24]byte
	r := NewEncryptingReader(strings.NewReader(""), c, nonce, 0)

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	// Should produce only the header (no data blocks for empty plaintext)
	if len(out) < cipher.FileHeaderSize {
		t.Errorf("output too short: %d < header %d", len(out), cipher.FileHeaderSize)
	}
	if string(out[:cipher.FileMagicSize]) != cipher.FileMagic {
		t.Errorf("missing file magic at header start")
	}
	_ = out[:cipher.FileHeaderSize] // verify at least header exists
}

func TestEncryptingReader_SingleBlockRoundTrip(t *testing.T) {
	c, _ := cipher.NewRcloneCipher("password", "salt")
	var nonce [24]byte
	// Use crypto/rand to fill nonce for production, but deterministic for test
	copy(nonce[:], []byte("012345678901234567890123"))

	plaintext := []byte("hello rclone encrypt reader")
	r := NewEncryptingReader(bytes.NewReader(plaintext), c, nonce, int64(len(plaintext)))

	encrypted, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	// Verify header
	if len(encrypted) < cipher.FileHeaderSize {
		t.Fatalf("encrypted data too short: %d", len(encrypted))
	}
	if string(encrypted[:cipher.FileMagicSize]) != cipher.FileMagic {
		t.Errorf("bad magic")
	}
	if !bytes.Equal(encrypted[cipher.FileMagicSize:cipher.FileHeaderSize], nonce[:]) {
		t.Errorf("nonce mismatch in header")
	}

	// Verify block: strip header, decrypt block
	blockData := encrypted[cipher.FileHeaderSize:]
	decrypted, err := c.DecryptBlock(blockData, 0, nonce)
	if err != nil {
		t.Fatalf("DecryptBlock failed: %v", err)
	}
	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("round-trip mismatch: got %q, want %q", string(decrypted), string(plaintext))
	}
}

func TestEncryptingReader_MultiBlock(t *testing.T) {
	c, _ := cipher.NewRcloneCipher("password", "salt")
	var nonce [24]byte
	copy(nonce[:], []byte("abcdefghijklmnopqrstuvwx"))

	// Three full blocks of data
	plaintext := make([]byte, cipher.BlockDataSize*3)
	for i := range plaintext {
		plaintext[i] = byte(i % 251)
	}

	r := NewEncryptingReader(bytes.NewReader(plaintext), c, nonce, int64(len(plaintext)))
	encrypted, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	// Verify header + 3 blocks
	expectedSize := cipher.FileHeaderSize + cipher.BlockSize*3
	if len(encrypted) != expectedSize {
		t.Errorf("expected %d bytes, got %d", expectedSize, len(encrypted))
	}

	// Decrypt each block
	for i := 0; i < 3; i++ {
		start := cipher.FileHeaderSize + i*cipher.BlockSize
		block := encrypted[start : start+cipher.BlockSize]
		decrypted, err := c.DecryptBlock(block, uint64(i), nonce)
		if err != nil {
			t.Fatalf("DecryptBlock block %d failed: %v", i, err)
		}
		expected := plaintext[i*cipher.BlockDataSize : (i+1)*cipher.BlockDataSize]
		if !bytes.Equal(decrypted, expected) {
			t.Errorf("block %d mismatch", i)
		}
	}
}

func TestEncryptingReader_PartialBlock(t *testing.T) {
	c, _ := cipher.NewRcloneCipher("password", "salt")
	var nonce [24]byte
	copy(nonce[:], []byte("123456789012345678901234"))

	plaintext := []byte("small")
	r := NewEncryptingReader(bytes.NewReader(plaintext), c, nonce, int64(len(plaintext)))
	encrypted, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	// Verify: header + 1 block (with partial data)
	expectedSize := cipher.FileHeaderSize + cipher.BlockHeaderSize + len(plaintext)
	if len(encrypted) != expectedSize {
		t.Errorf("expected %d bytes, got %d", expectedSize, len(encrypted))
	}

	decrypted, err := c.DecryptBlock(encrypted[cipher.FileHeaderSize:], 0, nonce)
	if err != nil {
		t.Fatalf("DecryptBlock failed: %v", err)
	}
	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("round-trip mismatch: got %q, want %q", string(decrypted), string(plaintext))
	}
}

func TestEncryptingReader_ReadSmallBuffer(t *testing.T) {
	c, _ := cipher.NewRcloneCipher("password", "salt")
	var nonce [24]byte
	copy(nonce[:], []byte("123456789012345678901234"))

	plaintext := bytes.Repeat([]byte("A"), cipher.BlockDataSize*2+100)
	r := NewEncryptingReader(bytes.NewReader(plaintext), c, nonce, int64(len(plaintext)))

	// Read in tiny chunks to exercise the partial buffer logic
	smallBuf := make([]byte, 17)
	var total []byte
	for {
		n, err := r.Read(smallBuf)
		if n > 0 {
			total = append(total, smallBuf[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Verify total size (last block may be partial)
	expectedSize := cipher.FileHeaderSize
	fullBlocks := len(plaintext) / cipher.BlockDataSize
	residue := len(plaintext) % cipher.BlockDataSize
	expectedSize += fullBlocks * cipher.BlockSize
	if residue > 0 {
		expectedSize += cipher.BlockHeaderSize + residue
	}
	if len(total) != expectedSize {
		t.Errorf("expected %d total bytes, got %d", expectedSize, len(total))
	}
}

// TestEncryptingReader_NonceCompatibility verifies the encrypting reader's
// output can be decrypted by the same cipher (self-consistency).
func TestEncryptingReader_NonceCompatibility(t *testing.T) {
	c, _ := cipher.NewRcloneCipher("password", "salt")
	var nonce [24]byte
	copy(nonce[:], []byte("nonce_test_1234567890!!"))

	plaintext := []byte("compatibility test data across blocks " + strings.Repeat("X", cipher.BlockDataSize*2))
	r := NewEncryptingReader(bytes.NewReader(plaintext), c, nonce, int64(len(plaintext)))

	encrypted, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	// Decrypt each block via the public API to verify round-trip
	var fileNonce [cipher.FileNonceSize]byte
	copy(fileNonce[:], encrypted[cipher.FileMagicSize:cipher.FileHeaderSize])

	var decrypted []byte
	pos := cipher.FileHeaderSize
	blockIdx := uint64(0)
	for pos < len(encrypted) {
		blockEnd := pos + cipher.BlockSize
		if blockEnd > len(encrypted) {
			blockEnd = len(encrypted)
		}
		block := encrypted[pos:blockEnd]

		plain, err := c.DecryptBlock(block, blockIdx, fileNonce)
		if err != nil {
			t.Fatalf("DecryptBlock failed at block %d: %v", blockIdx, err)
		}
		decrypted = append(decrypted, plain...)
		pos = blockEnd
		blockIdx++
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("decrypt mismatch: got %d bytes, want %d", len(decrypted), len(plaintext))
	}
}
