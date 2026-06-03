//go:build !nofuse

package fusefs

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/cipher"
	"github.com/yinzhenyu/skills/qrypt/internal/backend"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
)

func (fs *QryptFS) Read(path string, buff []byte, ofst int64, fh uint64) (n int) {
	logging.L.Infof("[FUSE] Read: path=%s, len=%d, offset=%d, fh=%d\n", path, len(buff), ofst, fh)
	node, errc := fs.lookupExtended(path, false)
	if errc != 0 {
		return 0
	}

	node.mu.RLock()
	nodeSize := node.size
	node.mu.RUnlock()
	if ofst >= nodeSize {
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

	startChunk := ofst / cipher.BlockDataSize
	endChunk := (ofst + int64(len(buff)) - 1) / cipher.BlockDataSize

	node.mu.Lock()
	if ofst == (node.lastReadBlock+1)*cipher.BlockDataSize {
		node.readSeqCount++
	} else if ofst != node.lastReadBlock*cipher.BlockDataSize {
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
			chunkStart = ofst % cipher.BlockDataSize
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

func (fs *QryptFS) prefetch(n *Node, startChunk uint64) {
	select {
	case fs.prefetchSem <- struct{}{}:
	default:
		return
	}
	defer func() { <-fs.prefetchSem }()

	n.mu.RLock()
	fileSize := n.size
	n.mu.RUnlock()

	seenBatches := make(map[uint64]bool)
	for i := uint64(0); i < FetchBatchBlocks/4; i++ {
		target := startChunk + i
		if int64(target)*cipher.BlockDataSize >= fileSize {
			break
		}
		batchIdx := target / FetchBatchBlocks
		if seenBatches[batchIdx] {
			continue
		}

		firstInBatch := batchIdx * FetchBatchBlocks
		if fs.cacheMgr != nil {
			if ok, _ := fs.cacheMgr.HasChunk(n.fid, int64(firstInBatch)); ok {
				seenBatches[batchIdx] = true
				continue
			}
		}
		mKey := fmt.Sprintf("%s_%d", n.fid, firstInBatch)
		if fs.memCache.Contains(mKey) {
			seenBatches[batchIdx] = true
			continue
		}

		seenBatches[batchIdx] = true
		go func(idx uint64) {
			if n.IsCancelled() {
				return
			}
			fs.getDecryptedChunk(n, idx)
		}(firstInBatch)
	}
}

func (fs *QryptFS) getDecryptedChunk(n *Node, idx uint64) ([]byte, error) {
	mKey := fmt.Sprintf("%s_%d", n.fid, idx)

	if v, ok := fs.memCache.Get(mKey); ok {
		return v, nil
	}

	if fs.cacheMgr != nil {
		data, err := fs.cacheMgr.GetChunk(n.fid, int64(idx))
		if err == nil && len(data) > 0 {
			fs.memCache.Add(mKey, data)
			return data, nil
		}
	}

	if strings.HasPrefix(n.fid, "local_") {
		return make([]byte, 0, cipher.BlockDataSize), nil
	}

	batchIdx := idx / FetchBatchBlocks
	batchKey := fmt.Sprintf("%s_batch_%d", n.fid, batchIdx)

	actual, loaded := fs.fetchingChunks.LoadOrStore(batchKey, make(chan struct{}))
	if loaded {
		<-actual.(chan struct{})
		if v, ok := fs.memCache.Get(mKey); ok {
			return v, nil
		}
		if fs.cacheMgr != nil {
			data, err := fs.cacheMgr.GetChunk(n.fid, int64(idx))
			if err == nil && len(data) > 0 {
				return data, nil
			}
		}
		logging.L.Warnf("getDecryptedChunk: data not found after concurrent fetch for %s chunk %d\n", n.fid, idx)
		return nil, fmt.Errorf("data not found after concurrent fetch")
	}

	defer func() {
		close(actual.(chan struct{}))
		fs.fetchingChunks.Delete(batchKey)
	}()

	if err := fs.fetchBatch(n, batchIdx); err != nil {
		logging.L.Errorf("fetchBatch: batch fetch failed for %s batch=%d: %v\n", n.fid, batchIdx, err)
		return nil, err
	}

	if v, ok := fs.memCache.Get(mKey); ok {
		return v, nil
	}
	if fs.cacheMgr != nil {
		data, err := fs.cacheMgr.GetChunk(n.fid, int64(idx))
		if err == nil && len(data) > 0 {
			fs.memCache.Add(mKey, data)
			return data, nil
		}
	}
	logging.L.Warnf("getDecryptedChunk: chunk %s_%d not in memCache or disk after fetchBatch succeeded\n", n.fid, idx)
	return nil, fmt.Errorf("chunk %s_%d lost from cache after fetchBatch", n.fid, idx)
}

func (fs *QryptFS) fetchBatch(n *Node, batchIdx uint64) error {
	n.mu.RLock()
	encSize := n.encSize
	fid := n.fid
	n.mu.RUnlock()

	entry := nodeToEntry(n)

	startBlock := batchIdx * FetchBatchBlocks
	var pStart, pEnd int64
	if batchIdx == 0 {
		pStart = 0
		pEnd = int64(cipher.FileHeaderSize) + int64(FetchBatchBlocks)*int64(cipher.BlockSize) - 1
	} else {
		pStart = int64(cipher.FileHeaderSize) + int64(startBlock)*int64(cipher.BlockSize)
		pEnd = pStart + int64(FetchBatchBlocks)*int64(cipher.BlockSize) - 1
	}
	if pEnd >= encSize {
		pEnd = encSize - 1
	}

	endBlock := startBlock + FetchBatchBlocks - 1
	batchSize := pEnd - pStart + 1
	rc, err := fs.drv.Read(context.Background(), entry, pStart, batchSize)
	if err != nil {
		if strings.Contains(err.Error(), "416") {
			return nil
		}
		// If file is not found (404 concurrency race), the file was likely
		// replaced by a concurrent upload. Retry with the latest fid.
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "21001") {
			n.mu.RLock()
			newFid := n.fid
			n.mu.RUnlock()
			if newFid != fid {
				logging.L.Debugf("fetchBatch: retrying %s with updated fid %s (was %s)\n", fid, newFid, fid)
				n.mu.RLock()
				encSize = n.encSize
				n.mu.RUnlock()
				entry.ID = newFid
				rc, err = fs.drv.Read(context.Background(), entry, pStart, batchSize)
				if err == nil {
					goto processBatch
				}
			}
		}
		logging.L.Errorf("fetchBatch: Read failed for %s: %v\n", fid, err)
		return err
	}

processBatch:
	defer rc.Close()

	if pStart == 0 {
		header := make([]byte, cipher.FileHeaderSize)
		_, errHeader := io.ReadFull(rc, header)
		if errHeader == nil && string(header[:len(cipher.FileMagic)]) == cipher.FileMagic {
			if !n.hasNonce {
				n.mu.Lock()
				copy(n.fileNonce[:], header[len(cipher.FileMagic):])
				n.hasNonce = true
				n.mu.Unlock()
			}
		}
	}

	for i := startBlock; i <= endBlock; i++ {
		encBlock := make([]byte, cipher.BlockSize)
		nRead, err := io.ReadFull(rc, encBlock)
		if err != nil {
			if err == io.EOF {
				break
			}
			if err == io.ErrUnexpectedEOF {
				if nRead > cipher.BlockHeaderSize {
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

		if fs.cacheMgr != nil {
			fs.cacheMgr.PutChunk(n.fid, int64(i), decBlock, false)
		}
		fs.memCache.Add(fmt.Sprintf("%s_%d", n.fid, i), decBlock)
	}

	return nil
}

func nodeToEntry(n *Node) backend.Entry {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return backend.Entry{
		ID:   n.fid,
		Name: n.name,
		Size: n.encSize,
	}
}

func (fs *QryptFS) Statfs(path string, stat *fuse.Statfs_t) (errc int) {
	stat.Bsize = 4096
	stat.Frsize = 4096
	stat.Blocks = 1024 * 1024 * 1024
	stat.Bfree = 1024 * 1024 * 512
	stat.Bavail = 1024 * 1024 * 512
	return 0
}
