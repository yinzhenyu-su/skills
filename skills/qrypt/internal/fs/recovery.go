//go:build !nofuse

package fs

import (
	"path/filepath"
	"strings"
	"time"
)

func (fs *QryptFS) recoverDirtyFiles() {
	if fs.cacheMgr == nil {
		return
	}
	nodes := fs.cacheMgr.GetPendingNodes()

	for _, f := range nodes {
		n, errc := fs.lookupExtended(f.Path, true)
		if errc != 0 {
			if !strings.HasPrefix(f.Fid, "local_") && f.LocalPath == "" {
				fs.cacheMgr.RemovePendingNode(f.Path)
				continue
			}

			if strings.HasPrefix(f.Fid, "local_") {
				if f.LocalPath != "" && fs.staging != nil {
					if _, sErr := fs.staging.FileSize(f.LocalPath); sErr != nil {
						fs.cacheMgr.RemovePendingNode(f.Path)
						continue
					}
				}
				parentPath := filepath.Dir(f.Path)
				_, parentErr := fs.lookupExtended(parentPath, true)
				if parentErr != 0 {
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
				continue
			}
			continue
		}

		n.mu.Lock()
		if !strings.HasPrefix(n.fid, "local_") && n.localPath == "" {
			n.isDirty = false
			n.syncQueued = false
			n.mu.Unlock()
			fs.cacheMgr.RemovePendingNode(f.Path)
			continue
		}

		if n.localPath != "" && fs.staging != nil {
			if _, sErr := fs.staging.FileSize(n.localPath); sErr != nil {
				n.isDirty = false
				n.syncQueued = false
				n.mu.Unlock()
				fs.cacheMgr.RemovePendingNode(f.Path)
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
	}
}
