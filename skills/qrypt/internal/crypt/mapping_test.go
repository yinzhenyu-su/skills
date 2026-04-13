package crypt

import "testing"

func TestEncryptedSize(t *testing.T) {
	tests := []struct {
		originalSize int64
		expectedSize int64
	}{
		{0, 0},
		{1, 1 + NonceSize + TagSize},
		{ChunkSize, ChunkSize + NonceSize + TagSize},
		{ChunkSize + 1, ChunkSize + 1 + 2*(NonceSize+TagSize)},
		{2 * ChunkSize, 2*ChunkSize + 2*(NonceSize+TagSize)},
	}

	for _, tt := range tests {
		got := EncryptedSize(tt.originalSize)
		if got != tt.expectedSize {
			t.Errorf("EncryptedSize(%d) = %d; want %d", tt.originalSize, got, tt.expectedSize)
		}
	}
}

func TestLogicalToPhysical(t *testing.T) {
	chunkIdx, offset := LogicalToPhysical(0)
	if chunkIdx != 0 || offset != 0 {
		t.Errorf("LogicalToPhysical(0) = (%d, %d); want (0, 0)", chunkIdx, offset)
	}

	chunkIdx, offset = LogicalToPhysical(ChunkSize)
	if chunkIdx != 1 || offset != 0 {
		t.Errorf("LogicalToPhysical(%d) = (%d, %d); want (1, 0)", ChunkSize, chunkIdx, offset)
	}

	chunkIdx, offset = LogicalToPhysical(ChunkSize + 10)
	if chunkIdx != 1 || offset != 10 {
		t.Errorf("LogicalToPhysical(%d) = (%d, %d); want (1, 10)", ChunkSize+10, chunkIdx, offset)
	}
}
