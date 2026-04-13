package crypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

const (
	ChunkSize     = 64 * 1024 // 64KB 明文块
	NonceSize     = 12        // AES-GCM 标准 Nonce 大小
	TagSize       = 16        // AES-GCM 标准 Tag 大小
	EncChunkSize  = ChunkSize + NonceSize + TagSize
)

// EncryptChunk 加密一个 64KB 的分块
func EncryptChunk(plaintext []byte, key []byte) ([]byte, error) {
	if len(plaintext) > ChunkSize {
		return nil, errors.New("plaintext too large")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// DecryptChunk 解密一个加密分块
func DecryptChunk(ciphertext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, encryptedData := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}
