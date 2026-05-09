package vfs

import (
	"fmt"
	"io"
	"runtime/debug"
	"strings"

	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/crypt"
	"github.com/yinzhenyu/skills/qrypt/internal/driver"
)

// Read 读取文件内容
func (fs *QryptFS) Read(path string, buff []byte, ofst int64, fh uint64) (n int) {
	driver.Log.Infof("[FUSE] Read: path=%s, len=%d, offset=%d, fh=%d\n", path, len(buff), ofst, fh)
	node, errc := fs.lookupExtended(path, false)
	if errc != 0 {
		return 0
	}

	if ofst >= node.size {
		return 0
	}
	node.mu.RLock()
	localPath := node.localPath
	node.mu.RUnlock()
	if localPath != "" && fs.staging != nil {
		nRead, err := fs.staging.ReadAt(localPath, buff, ofst)
		if err == io.EOF {
			return nRead
		}
		if err == nil || nRead > 0 {
			return nRead
		}
	}

	startChunk := ofst / crypt.BlockDataSize
	endChunk := (ofst + int64(len(buff)) - 1) / crypt.BlockDataSize

	node.mu.Lock()
	if ofst == (node.lastReadBlock+1)*crypt.BlockDataSize {
		node.readSeqCount++
	} else if ofst != node.lastReadBlock*crypt.BlockDataSize {
		node.readSeqCount = 0
	}
	node.lastReadBlock = endChunk
	seqCount := node.readSeqCount
	node.mu.Unlock()

	if seqCount >= 2 {
		go fs.prefetch(node, uint64(endChunk+1))
	}

	totalRead := 0
	for i := startChunk; i <= endChunk; i++ {
		chunkData, err := fs.getDecryptedChunk(node, uint64(i))
		if err != nil {
			break
		}

		chunkStart := int64(0)
		if i == startChunk {
			chunkStart = ofst % crypt.BlockDataSize
		}

		chunkEnd := int64(len(chunkData))
		remainingInBuff := int64(len(buff)) - int64(totalRead)
		if chunkEnd-chunkStart > remainingInBuff {
			chunkEnd = chunkStart + remainingInBuff
		}

		if chunkStart >= int64(len(chunkData)) {
			continue
		}

		nCopied := copy(buff[totalRead:], chunkData[chunkStart:chunkEnd])
		totalRead += nCopied

		if int64(totalRead) >= int64(len(buff)) {
			break
		}
	}

	return totalRead
}

func (fs *QryptFS) prefetch(n *node, startChunk uint64) {
	defer func() {
		if r := recover(); r != nil {
			driver.Log.Errorf("PANIC in prefetch: %v\n%s\n", r, debug.Stack())
		}
	}()
	for i := uint64(0); i < FetchBatchBlocks/4; i++ {
		target := startChunk + i
		if int64(target)*crypt.BlockDataSize >= n.size {
			break
		}
		if fs.cache != nil {
			if ok, _ := fs.cache.HasChunk(n.fid, int64(target)); ok {
				continue
			}
		}
		mKey := fmt.Sprintf("%s_%d", n.fid, target)
		if fs.memCache.Contains(mKey) {
			continue
		}

		go func(idx uint64) {
			defer func() {
				if r := recover(); r != nil {
					driver.Log.Errorf("PANIC in prefetch goroutine: %v\n%s\n", r, debug.Stack())
				}
			}()
			_, _ = fs.getDecryptedChunk(n, idx)
		}(target)
	}
}

func (fs *QryptFS) getDecryptedChunk(n *node, idx uint64) ([]byte, error) {
	mKey := fmt.Sprintf("%s_%d", n.fid, idx)

	if v, ok := fs.memCache.Get(mKey); ok {
		return v, nil
	}

	if fs.cache != nil {
		data, err := fs.cache.GetChunk(n.fid, int64(idx))
		if err == nil && len(data) > 0 {
			fs.memCache.Add(mKey, data)
			return data, nil
		}
	}

	if strings.HasPrefix(n.fid, "local_") {
		return make([]byte, 0, crypt.BlockDataSize), nil
	}

	batchIdx := idx / FetchBatchBlocks
	batchKey := fmt.Sprintf("%s_batch_%d", n.fid, batchIdx)

	actual, loaded := fs.fetching.LoadOrStore(batchKey, make(chan struct{}))
	if loaded {
		<-actual.(chan struct{})
		if v, ok := fs.memCache.Get(mKey); ok {
			return v, nil
		}
		if fs.cache != nil {
			data, err := fs.cache.GetChunk(n.fid, int64(idx))
			if err == nil && len(data) > 0 {
				return data, nil
			}
		}
		return nil, fmt.Errorf("data not found after concurrent fetch")
	}

	defer func() {
		close(actual.(chan struct{}))
		fs.fetching.Delete(batchKey)
	}()

	if err := fs.fetchBatch(n, batchIdx); err != nil {
		return nil, err
	}

	if v, ok := fs.memCache.Get(mKey); ok {
		return v, nil
	}
	return make([]byte, 0, crypt.BlockDataSize), nil
}

func (fs *QryptFS) fetchBatch(n *node, batchIdx uint64) error {
	url, err := fs.driver.GetDownloadURL(n.fid)
	if err != nil {
		return err
	}

	startBlock := batchIdx * FetchBatchBlocks
	endBlock := startBlock + FetchBatchBlocks - 1

	pStart := int64(crypt.FileHeaderSize) + int64(startBlock)*int64(crypt.BlockSize)
	if pStart >= n.encSize {
		pStart = 0
	}
	pEnd := pStart + int64(FetchBatchBlocks)*int64(crypt.BlockSize) - 1
	if pStart == 0 {
		pEnd += int64(crypt.FileHeaderSize)
	}
	if pEnd >= n.encSize {
		pEnd = n.encSize - 1
	}

	driver.Log.Infof("Batch Fetch: '%s' Blocks %d-%d\n", n.name, startBlock, endBlock)

	rc, err := fs.driver.DownloadChunk(url, pStart, pEnd)
	if err != nil {
		if strings.Contains(err.Error(), "416") {
			return nil
		}
		return err
	}
	defer rc.Close()

	if !n.hasNonce {
		hrc, err := fs.driver.DownloadChunk(url, 0, int64(crypt.FileHeaderSize)-1)
		if err == nil {
			header := make([]byte, crypt.FileHeaderSize)
			_, errRead := io.ReadFull(hrc, header)
			hrc.Close()
			if errRead == nil && string(header[:len(crypt.FileMagic)]) == crypt.FileMagic {
				n.mu.Lock()
				copy(n.fileNonce[:], header[len(crypt.FileMagic):])
				n.hasNonce = true
				n.mu.Unlock()
			}
		}
	}

	if pStart == 0 {
		header := make([]byte, crypt.FileHeaderSize)
		_, errHeader := io.ReadFull(rc, header)
		if errHeader == nil && string(header[:len(crypt.FileMagic)]) == crypt.FileMagic {
			if !n.hasNonce {
				n.mu.Lock()
				copy(n.fileNonce[:], header[len(crypt.FileMagic):])
				n.hasNonce = true
				n.mu.Unlock()
			}
		}
	}

	for i := startBlock; i <= endBlock; i++ {
		encBlock := make([]byte, crypt.BlockSize)
		nRead, err := io.ReadFull(rc, encBlock)
		if err != nil {
			if err == io.EOF {
				break
			}
			if err == io.ErrUnexpectedEOF {
				if nRead > crypt.BlockHeaderSize {
					encBlock = encBlock[:nRead]
				} else {
					break
				}
			} else {
				return err
			}
		}

		n.mu.RLock()
		nonce := n.fileNonce
		n.mu.RUnlock()

		decBlock, err := fs.cipher.DecryptBlock(encBlock, uint64(i), nonce)
		if err != nil {
			return fmt.Errorf("decryption failed for block %d: %v", i, err)
		}

		if fs.cache != nil {
			_ = fs.cache.PutChunk(n.fid, int64(i), decBlock, false)
		}
		fs.memCache.Add(fmt.Sprintf("%s_%d", n.fid, i), decBlock)
	}

	return nil
}

// Statfs 返回文件系统统计信息
func (fs *QryptFS) Statfs(path string, stat *fuse.Statfs_t) (errc int) {
	stat.Bsize = 4096
	stat.Frsize = 4096
	stat.Blocks = 1024 * 1024 * 1024 // 虚拟 4TB
	stat.Bfree = 1024 * 1024 * 512
	stat.Bavail = 1024 * 1024 * 512
	return 0
}
