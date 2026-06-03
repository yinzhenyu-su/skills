package cipher

type Cipher interface {
	EncryptSegment(plaintext string) string
	DecryptSegment(encrypted string) (string, error)
	EncryptBlock(plaintext []byte, blockIndex uint64, fileNonce [24]byte) ([]byte, error)
	DecryptBlock(ciphertext []byte, blockIndex uint64, fileNonce [24]byte) ([]byte, error)
	EncryptedSize(size int64) int64
	DecryptedSize(size int64) (int64, error)
	GenerateRandomNonce() ([24]byte, error)
}
