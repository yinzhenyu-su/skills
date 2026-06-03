package qrypt

import (
	"context"
	"io"
	"time"
)

const (
	FileMagic       = "RCLONE\x00\x00"
	FileMagicSize   = len(FileMagic)
	FileNonceSize   = 24
	FileHeaderSize  = FileMagicSize + FileNonceSize
	BlockHeaderSize = 16
	BlockDataSize   = 64 * 1024
	BlockSize       = BlockHeaderSize + BlockDataSize
)

type Entry struct {
	ID       string
	ParentID string
	Name     string
	IsDir    bool
	Size     int64
	ModTime  time.Time
	Extra    any
}

type Driver interface {
	Init(ctx context.Context) error
	Drop(ctx context.Context) error
	List(ctx context.Context, parentID string) ([]Entry, error)
	Read(ctx context.Context, entry Entry, offset, size int64) (io.ReadCloser, error)
}

type Writer interface {
	Mkdir(ctx context.Context, parentID, name string) (Entry, error)
	Move(ctx context.Context, entry Entry, dstParentID string) error
	Rename(ctx context.Context, entry Entry, newName string) error
	Remove(ctx context.Context, entry Entry) error
}

type Uploader interface {
	Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (Entry, error)
}

type PathResolver interface {
	ResolvePath(ctx context.Context, path string) (string, error)
}

type mtimeKey struct{}

func WithMtime(ctx context.Context, mtime time.Time) context.Context {
	return context.WithValue(ctx, mtimeKey{}, mtime)
}

func MtimeFromContext(ctx context.Context) (time.Time, bool) {
	mtime, ok := ctx.Value(mtimeKey{}).(time.Time)
	return mtime, ok
}

type Cipher interface {
	EncryptSegment(plain string) string
	DecryptSegment(cipher string) (string, error)
	EncryptBlock(plaintext []byte, blockIndex uint64, fileNonce [FileNonceSize]byte) ([]byte, error)
	DecryptBlock(ciphertext []byte, blockIndex uint64, fileNonce [FileNonceSize]byte) ([]byte, error)
	EncryptedSize(plainSize int64) int64
	DecryptedSize(cipherSize int64) (int64, error)
	GenerateRandomNonce() ([FileNonceSize]byte, error)
}
