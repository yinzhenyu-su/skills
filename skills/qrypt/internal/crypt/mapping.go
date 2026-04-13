package crypt

// LogicalToPhysical 将明文偏移量映射到密文偏移量和分块索引
func LogicalToPhysical(logicalOffset int64) (chunkIndex int64, chunkOffset int64) {
	chunkIndex = logicalOffset / ChunkSize
	chunkOffset = logicalOffset % ChunkSize
	return
}

// PhysicalRange 获取指定明文范围对应的密文范围
func PhysicalRange(start, end int64) (pStart, pEnd int64) {
	startIndex, _ := LogicalToPhysical(start)
	endIndex, _ := LogicalToPhysical(end)

	pStart = startIndex * EncChunkSize
	pEnd = (endIndex+1)*EncChunkSize - 1
	return
}

// EncryptedSize 根据原始文件大小计算加密后的大小
func EncryptedSize(originalSize int64) int64 {
	if originalSize == 0 {
		return 0
	}
	numChunks := (originalSize + ChunkSize - 1) / ChunkSize
	// 只有最后一个块可能不满，但我们始终为其分配完整的 Tag 和 Nonce 空间
	// 为了简化，假设所有块加密后都增加 (NonceSize + TagSize)
	return originalSize + numChunks*(NonceSize+TagSize)
}
