//go:build !nofuse

package fusefs

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/logging"
)

func (fs *QryptFS) recoverDirtyFiles() {
	if fs.cacheMgr == nil {
		return
	}
	nodes := fs.cacheMgr.GetPendingNodes()
	total := len(nodes)
	if total == 0 {
		return
	}

	logging.L.Infof("recoverDirtyFiles: found %d pending dirty nodes\n", total)
	var (
		nRecovered int
		nCleaned   int
		nSkipped   int
	)

	for _, f := range nodes {
		n, errc := fs.lookupExtended(f.Path, true)
		if errc != 0 {
			if !strings.HasPrefix(f.Fid, "local_") && f.LocalPath == "" {
				// Remote-fid file with no staging → was already uploaded.
				fs.cacheMgr.RemovePendingNode(f.Path)
				nCleaned++
				logging.L.Debugf("recoverDirtyFiles: cleaned record for %s (fid=%s, already uploaded)\n", f.Path, f.Fid)
				continue
			}

			if strings.HasPrefix(f.Fid, "local_") {
				if f.LocalPath != "" && fs.staging != nil {
					if _, sErr := fs.staging.FileSize(f.LocalPath); sErr != nil {
						fs.cacheMgr.RemovePendingNode(f.Path)
						nCleaned++
						logging.L.Debugf("recoverDirtyFiles: cleaned record for %s (staging file missing)\n", f.Path)
						continue
					}
				}
				parentPath := filepath.Dir(f.Path)
				_, parentErr := fs.lookupExtended(parentPath, true)
				if parentErr != 0 {
					nSkipped++
					logging.L.Debugf("recoverDirtyFiles: skipped %s (parent not found)\n", f.Path)
					continue
				}

				newNode := &Node{
					fid:               f.Fid,
					parentFid:         f.ParentFid,
					name:              f.Name,
					currentPath:       f.Path,
					size:              f.Size,
					isFolder:          f.IsFolder,
					isDirty:           true,
					localPath:         f.LocalPath,
					source:            "local",
					lastMetadataCheck: time.Now(),
					baseServerMtime:   f.BaseServerMtime,
					baseServerSize:    f.BaseServerSize,
					uploadID:          f.UploadID,
					lastPart:          f.LastPart,
				}
				if len(f.Nonce) == 24 {
					copy(newNode.fileNonce[:], f.Nonce)
					newNode.hasNonce = true
				}
				fs.storeNode(f.Path, newNode)
				newNode.mu.Lock()
				newNode.syncQueued = true
				newNode.mu.Unlock()
				fs.enqueueNode(newNode)
				nRecovered++
				logging.L.Infof("recoverDirtyFiles: recovered %s (staging=%s, size=%d)\n", f.Path, f.LocalPath, f.Size)
				continue
			}
			nSkipped++
			logging.L.Debugf("recoverDirtyFiles: skipped %s (fid=%s, lookup failed)\n", f.Path, f.Fid)
			continue
		}

		n.mu.Lock()
		if !strings.HasPrefix(n.fid, "local_") && n.localPath == "" {
			n.isDirty = false
			n.syncQueued = false
			n.mu.Unlock()
			fs.cacheMgr.RemovePendingNode(f.Path)
			nCleaned++
			logging.L.Debugf("recoverDirtyFiles: cleaned record for %s (fid=%s, no staging)\n", f.Path, n.fid)
			continue
		}

		if n.localPath != "" && fs.staging != nil {
			if _, sErr := fs.staging.FileSize(n.localPath); sErr != nil {
				n.isDirty = false
				n.syncQueued = false
				n.mu.Unlock()
				fs.cacheMgr.RemovePendingNode(f.Path)
				nCleaned++
				logging.L.Debugf("recoverDirtyFiles: cleaned record for %s (staging file gone)\n", f.Path)
				continue
			}
		}

		n.isDirty = true
		n.syncQueued = true
		if f.UploadID != "" {
			n.uploadID = f.UploadID
		}
		if f.LastPart > 0 {
			n.lastPart = f.LastPart
		}
		n.mu.Unlock()
		fs.enqueueNode(n)
		nRecovered++
		logging.L.Infof("recoverDirtyFiles: re-enqueued %s (fid=%s, size=%d)\n", f.Path, n.fid, f.Size)
	}

	logging.L.Infof("recoverDirtyFiles: done — %d recovered, %d cleaned, %d skipped (of %d total)\n",
		nRecovered, nCleaned, nSkipped, total)
}
