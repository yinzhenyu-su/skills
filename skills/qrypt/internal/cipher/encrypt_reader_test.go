package cipher

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"golang.org/x/crypto/nacl/secretbox"
)

func TestNewEncryptingReader(t *testing.T) {
	c, _ := NewRcloneCipher("password", "salt")
	var nonce [24]byte
	r := NewEncryptingReader(strings.NewReader("hello"), c, nonce, 5)
	if r == nil {
		t.Fatal("expected non-nil reader")
	}
}

func TestEncryptingReader_Read_Empty(t *testing.T) {
	c, _ := NewRcloneCipher("password", "salt")
	var nonce [24]byte
	r := NewEncryptingReader(strings.NewReader(""), c, nonce, 0)

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	// Should produce only the header (no data blocks for empty plaintext)
	if len(out) < FileHeaderSize {
		t.Errorf("output too short: %d < header %d", len(out), FileHeaderSize)
	}
	if string(out[:FileMagicSize]) != FileMagic {
		t.Errorf("missing file magic at header start")
	}
	_ = out[:FileHeaderSize] // verify at least header exists
}

func TestEncryptingReader_SingleBlockRoundTrip(t *testing.T) {
	c, _ := NewRcloneCipher("password", "salt")
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
	if len(encrypted) < FileHeaderSize {
		t.Fatalf("encrypted data too short: %d", len(encrypted))
	}
	if string(encrypted[:FileMagicSize]) != FileMagic {
		t.Errorf("bad magic")
	}
	if !bytes.Equal(encrypted[FileMagicSize:FileHeaderSize], nonce[:]) {
		t.Errorf("nonce mismatch in header")
	}

	// Verify block: strip header, decrypt block
	blockData := encrypted[FileHeaderSize:]
	decrypted, err := c.DecryptBlock(blockData, 0, nonce)
	if err != nil {
		t.Fatalf("DecryptBlock failed: %v", err)
	}
	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("round-trip mismatch: got %q, want %q", string(decrypted), string(plaintext))
	}
}

func TestEncryptingReader_MultiBlock(t *testing.T) {
	c, _ := NewRcloneCipher("password", "salt")
	var nonce [24]byte
	copy(nonce[:], []byte("abcdefghijklmnopqrstuvwx"))

	// Three full blocks of data
	plaintext := make([]byte, BlockDataSize*3)
	for i := range plaintext {
		plaintext[i] = byte(i % 251)
	}

	r := NewEncryptingReader(bytes.NewReader(plaintext), c, nonce, int64(len(plaintext)))
	encrypted, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	// Verify header + 3 blocks
	expectedSize := FileHeaderSize + BlockSize*3
	if len(encrypted) != expectedSize {
		t.Errorf("expected %d bytes, got %d", expectedSize, len(encrypted))
	}

	// Decrypt each block
	for i := 0; i < 3; i++ {
		start := FileHeaderSize + i*BlockSize
		block := encrypted[start : start+BlockSize]
		decrypted, err := c.DecryptBlock(block, uint64(i), nonce)
		if err != nil {
			t.Fatalf("DecryptBlock block %d failed: %v", i, err)
		}
		expected := plaintext[i*BlockDataSize : (i+1)*BlockDataSize]
		if !bytes.Equal(decrypted, expected) {
			t.Errorf("block %d mismatch", i)
		}
	}
}

func TestEncryptingReader_PartialBlock(t *testing.T) {
	c, _ := NewRcloneCipher("password", "salt")
	var nonce [24]byte
	copy(nonce[:], []byte("123456789012345678901234"))

	plaintext := []byte("small")
	r := NewEncryptingReader(bytes.NewReader(plaintext), c, nonce, int64(len(plaintext)))
	encrypted, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	// Verify: header + 1 block (with partial data)
	expectedSize := FileHeaderSize + BlockHeaderSize + len(plaintext)
	if len(encrypted) != expectedSize {
		t.Errorf("expected %d bytes, got %d", expectedSize, len(encrypted))
	}

	decrypted, err := c.DecryptBlock(encrypted[FileHeaderSize:], 0, nonce)
	if err != nil {
		t.Fatalf("DecryptBlock failed: %v", err)
	}
	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("round-trip mismatch: got %q, want %q", string(decrypted), string(plaintext))
	}
}

func TestEncryptingReader_ReadSmallBuffer(t *testing.T) {
	c, _ := NewRcloneCipher("password", "salt")
	var nonce [24]byte
	copy(nonce[:], []byte("123456789012345678901234"))

	plaintext := bytes.Repeat([]byte("A"), BlockDataSize*2+100)
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
	expectedSize := FileHeaderSize
	fullBlocks := len(plaintext) / BlockDataSize
	residue := len(plaintext) % BlockDataSize
	expectedSize += fullBlocks * BlockSize
	if residue > 0 {
		expectedSize += BlockHeaderSize + residue
	}
	if len(total) != expectedSize {
		t.Errorf("expected %d total bytes, got %d", expectedSize, len(total))
	}
}

// TestEncryptingReader_NonceCompatibility verifies the encrypting reader's
// output can be decrypted by the same cipher (self-consistency).
func TestEncryptingReader_NonceCompatibility(t *testing.T) {
	c, _ := NewRcloneCipher("password", "salt")
	var nonce [24]byte
	copy(nonce[:], []byte("nonce_test_1234567890!!"))

	plaintext := []byte("compatibility test data across blocks " + strings.Repeat("X", BlockDataSize*2))
	r := NewEncryptingReader(bytes.NewReader(plaintext), c, nonce, int64(len(plaintext)))

	encrypted, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	// Manually decode: read nonce from header, decrypt each block
	var fileNonce [24]byte
	copy(fileNonce[:], encrypted[FileMagicSize:FileHeaderSize])

	var decrypted []byte
	pos := FileHeaderSize
	blockIdx := uint64(0)
	for pos < len(encrypted) {
		blockEnd := pos + BlockSize
		if blockEnd > len(encrypted) {
			blockEnd = len(encrypted)
		}
		block := encrypted[pos:blockEnd]

		// Decrypt with secretbox directly to verify
		var blockNonce [24]byte
		copy(blockNonce[:], fileNonce[:])
		u := blockIdx
		for i := 0; i < 8 && u > 0; i++ {
			u += uint64(blockNonce[i])
			blockNonce[i] = byte(u)
			u >>= 8
		}

		plain, ok := secretbox.Open(nil, block, &blockNonce, &c.dataKey)
		if !ok {
			t.Fatalf("secretbox.Open failed at block %d", blockIdx)
		}
		decrypted = append(decrypted, plain...)
		pos = blockEnd
		blockIdx++
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("manual decrypt mismatch: got %d bytes, want %d", len(decrypted), len(plaintext))
	}
}
