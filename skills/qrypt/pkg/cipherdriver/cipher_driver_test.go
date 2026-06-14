package cipherdriver_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yinzhenyu/skills/qrypt/cipher"
	"github.com/yinzhenyu/skills/qrypt/internal/mockdrive"
	"github.com/yinzhenyu/skills/qrypt/pkg/cipherdriver"
)

func newTestDriver(t *testing.T) (*cipherdriver.CipherDriver, *mockdrive.MockDriver, cipher.Cipher) {
	t.Helper()
	raw := mockdrive.NewDriver()
	cp, err := cipher.NewRcloneCipher("test-password", "")
	require.NoError(t, err)
	cd := cipherdriver.New(raw, cp)
	return cd, raw, cp
}

// ---------------------------------------------------------------------------
// ReadBlock tests
// ---------------------------------------------------------------------------

func TestCipherDriver_ReadBlock_Block0(t *testing.T) {
	t.Parallel()

	cd, raw, cp := newTestDriver(t)
	ctx := context.Background()

	nonce, err := cp.GenerateRandomNonce()
	require.NoError(t, err)

	plaintext := []byte("hello world, this is test data for block 0")
	encBlock, err := cp.EncryptBlock(plaintext, 0, nonce)
	require.NoError(t, err)

	// Build file: magic + nonce + encrypted block.
	var encData bytes.Buffer
	encData.Write([]byte(cipher.FileMagic))
	encData.Write(nonce[:])
	encData.Write(encBlock)

	entry, err := raw.Put(ctx, "0", "encrypted-file", int64(encData.Len()), &encData)
	require.NoError(t, err)

	got, err := cd.ReadBlock(ctx, entry, 0)
	require.NoError(t, err)
	require.Equal(t, plaintext, got)

	// Second call — verifies nonce cache works.
	got2, err := cd.ReadBlock(ctx, entry, 0)
	require.NoError(t, err)
	require.Equal(t, plaintext, got2)
}

func TestCipherDriver_ReadBlock_BlockN(t *testing.T) {
	t.Parallel()

	cd, raw, cp := newTestDriver(t)
	ctx := context.Background()

	nonce, err := cp.GenerateRandomNonce()
	require.NoError(t, err)

	// Full BlockDataSize blocks so each encrypted block is exactly BlockSize bytes,
	// matching ReadBlock's fixed-offset layout.
	pt0 := bytes.Repeat([]byte("A"), cipher.BlockDataSize)
	pt1 := bytes.Repeat([]byte("B"), cipher.BlockDataSize)
	pt2 := bytes.Repeat([]byte("C"), cipher.BlockDataSize)

	enc0, err := cp.EncryptBlock(pt0, 0, nonce)
	require.NoError(t, err)
	enc1, err := cp.EncryptBlock(pt1, 1, nonce)
	require.NoError(t, err)
	enc2, err := cp.EncryptBlock(pt2, 2, nonce)
	require.NoError(t, err)

	var encData bytes.Buffer
	encData.Write([]byte(cipher.FileMagic))
	encData.Write(nonce[:])
	encData.Write(enc0)
	encData.Write(enc1)
	encData.Write(enc2)

	entry, err := raw.Put(ctx, "0", "encrypted-file", int64(encData.Len()), &encData)
	require.NoError(t, err)

	// Read block 0 — triggers nonce extraction from header.
	got0, err := cd.ReadBlock(ctx, entry, 0)
	require.NoError(t, err)
	require.Equal(t, pt0, got0)

	// Read block 2 — uses cached nonce.
	got2, err := cd.ReadBlock(ctx, entry, 2)
	require.NoError(t, err)
	require.Equal(t, pt2, got2)
}

func TestCipherDriver_ReadBlock_InvalidMagic(t *testing.T) {
	t.Parallel()

	cd, raw, cp := newTestDriver(t)
	ctx := context.Background()

	nonce, err := cp.GenerateRandomNonce()
	require.NoError(t, err)

	plaintext := []byte("test data")
	encBlock, err := cp.EncryptBlock(plaintext, 0, nonce)
	require.NoError(t, err)

	var encData bytes.Buffer
	encData.Write(bytes.Repeat([]byte{0}, cipher.FileMagicSize)) // wrong magic
	encData.Write(nonce[:])
	encData.Write(encBlock)

	entry, err := raw.Put(ctx, "0", "bad-file", int64(encData.Len()), &encData)
	require.NoError(t, err)

	_, err = cd.ReadBlock(ctx, entry, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid file magic")
}

func TestCipherDriver_ReadBlock_TruncatedHeader(t *testing.T) {
	t.Parallel()

	cd, raw, _ := newTestDriver(t)
	ctx := context.Background()

	shortData := make([]byte, cipher.FileHeaderSize-1)

	entry, err := raw.Put(ctx, "0", "short-file", int64(len(shortData)), bytes.NewReader(shortData))
	require.NoError(t, err)

	_, err = cd.ReadBlock(ctx, entry, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "header too short")
}

// ---------------------------------------------------------------------------
// List test
// ---------------------------------------------------------------------------

func TestCipherDriver_List(t *testing.T) {
	t.Parallel()

	cd, raw, cp := newTestDriver(t)
	ctx := context.Background()

	// Create dir with encrypted name in raw driver.
	dirName := "my-documents"
	encDirName := cp.EncryptSegment(dirName)
	dirEntry, err := raw.Mkdir(ctx, "0", encDirName)
	require.NoError(t, err)

	// Create file inside dir with encrypted name.
	innerFile := "readme.txt"
	encInner := cp.EncryptSegment(innerFile)
	_, err = raw.Put(ctx, dirEntry.ID, encInner, 5, bytes.NewReader([]byte("hello")))
	require.NoError(t, err)

	// Create file at root with encrypted name.
	rootFile := "notes.md"
	encRoot := cp.EncryptSegment(rootFile)
	_, err = raw.Put(ctx, "0", encRoot, 10, bytes.NewReader([]byte("some notes")))
	require.NoError(t, err)

	// Create entry with undecryptable name — should be skipped.
	_, err = raw.Mkdir(ctx, "0", "!!!invalid-base32-garbage!!!")
	require.NoError(t, err)

	// List root — should see dirName + rootFile, NOT the garbage entry.
	entries, err := cd.List(ctx, "0")
	require.NoError(t, err)
	require.Len(t, entries, 2)

	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name] = true
	}
	require.True(t, names[dirName], "should find %q", dirName)
	require.True(t, names[rootFile], "should find %q", rootFile)

	// List child directory.
	child, err := cd.List(ctx, dirEntry.ID)
	require.NoError(t, err)
	require.Len(t, child, 1)
	require.Equal(t, innerFile, child[0].Name)
}

// ---------------------------------------------------------------------------
// Mkdir test
// ---------------------------------------------------------------------------

func TestCipherDriver_Mkdir(t *testing.T) {
	t.Parallel()

	cd, raw, cp := newTestDriver(t)
	ctx := context.Background()

	folderName := "new-folder"
	entry, err := cd.Mkdir(ctx, "0", folderName)
	require.NoError(t, err)
	require.True(t, entry.IsDir)

	// CipherDriver.List should return the decrypted name.
	entries, err := cd.List(ctx, "0")
	require.NoError(t, err)

	foundDec := false
	for _, e := range entries {
		if e.Name == folderName {
			foundDec = true
			break
		}
	}
	require.True(t, foundDec, "new-folder should appear in listing")

	// Raw driver should store the encrypted name.
	rawEntries, err := raw.List(ctx, "0")
	require.NoError(t, err)

	encName := cp.EncryptSegment(folderName)
	foundRaw := false
	for _, e := range rawEntries {
		if e.Name == encName {
			foundRaw = true
			break
		}
	}
	require.True(t, foundRaw, "raw driver should store encrypted name")
}

// ---------------------------------------------------------------------------
// Put tests
// ---------------------------------------------------------------------------

func TestCipherDriver_Put(t *testing.T) {
	t.Parallel()

	cd, _, _ := newTestDriver(t)
	ctx := context.Background()

	plaintext := []byte("hello world")
	reader := bytes.NewReader(plaintext)

	entry, err := cd.Put(ctx, "0", "test.txt", int64(len(plaintext)), reader)
	require.NoError(t, err)
	require.NotEmpty(t, entry.ID, "entry should have an ID assigned")

	// ReadBlock should return the original plaintext.
	got, err := cd.ReadBlock(ctx, entry, 0)
	require.NoError(t, err)
	require.Equal(t, plaintext, got)

	// List should show "test.txt" with decrypted name.
	entries, err := cd.List(ctx, "0")
	require.NoError(t, err)

	found := false
	for _, e := range entries {
		if e.Name == "test.txt" {
			found = true
			break
		}
	}
	require.True(t, found, "test.txt should appear in listing")
}

func TestCipherDriver_PutThenReadBlock(t *testing.T) {
	t.Parallel()

	cd, _, _ := newTestDriver(t)
	ctx := context.Background()

	plaintext := []byte("immediate read test data")
	reader := bytes.NewReader(plaintext)

	entry, err := cd.Put(ctx, "0", "immediate.txt", int64(len(plaintext)), reader)
	require.NoError(t, err)

	// ReadBlock immediately after Put — proves nonce cached during Put.
	got, err := cd.ReadBlock(ctx, entry, 0)
	require.NoError(t, err)
	require.Equal(t, plaintext, got)
}
