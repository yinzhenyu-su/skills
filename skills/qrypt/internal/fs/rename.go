package fs

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
	"github.com/yinzhenyu/skills/qrypt/internal/log"
)

func (fs *QryptFS) Rename(oldPath string, newPath string) (errc int) {
	defer func() {
		if r := recover(); r != nil {
			log.L.Errorf("PANIC in Rename(%s->%s): %v\n", oldPath, newPath, r)
			errc = -fuse.EIO
		}
	}()
	if fs.IsShuttingDown() {
		log.L.Warnf("[SHUTDOWN] Rejecting Rename: %s -> %s\n", oldPath, newPath)
		return -fuse.EIO
	}
	log.L.Infof("[FUSE] Rename: oldPath=%s, newPath=%s\n", oldPath, newPath)

	oldNode, errc := fs.lookup(oldPath)
	if errc != 0 {
		return errc
	}

	oldParent := filepath.Dir(oldPath)
	newParent := filepath.Dir(newPath)
	newName := filepath.Base(newPath)
	isLocal := strings.HasPrefix(oldNode.fid, "local_")

	w, wOk := fs.drv.(drive.Writer)

	if oldParent != newParent {
		newParentNode, errc := fs.lookup(newParent)
		if errc != 0 {
			return errc
		}

		if !isLocal && wOk {
			moveEntry := drive.Entry{ID: oldNode.fid}
			var moveErr error
			for attempt := 0; attempt < 5; attempt++ {
				moveErr = w.Move(context.Background(), moveEntry, newParentNode.fid)
				if moveErr == nil {
					break
				}
				if errors.Is(moveErr, drive.ErrDirAlreadyExists) || strings.Contains(moveErr.Error(), "conflict") {
					time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
					continue
				}
				break
			}
			if moveErr != nil {
				return -fuse.EIO
			}
		}

		oldNode.mu.Lock()
		oldNode.parentFid = newParentNode.fid
		oldNode.mu.Unlock()
	}

	if !isLocal && wOk {
		encName := fs.cipher.EncryptSegment(newName)
		oldEncName := fs.cipher.EncryptSegment(oldNode.name)
		if encName != oldEncName {
			renameEntry := drive.Entry{ID: oldNode.fid}
			var renameErr error
			for attempt := 0; attempt < 5; attempt++ {
				renameErr = w.Rename(context.Background(), renameEntry, encName)
				if renameErr == nil {
					break
				}
				if errors.Is(renameErr, drive.ErrDirAlreadyExists) || strings.Contains(renameErr.Error(), "conflict") {
					time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
					continue
				}
				break
			}
			if renameErr != nil {
				return -fuse.EIO
			}
		}
	}

	oldNode.mu.Lock()
	oldNode.name = newName
	oldNode.mu.Unlock()

	fs.replaceNodePath(oldPath, newPath, oldNode)
	fs.persistPendingPath(oldPath, newPath, oldNode)

	if oldNode.isFolder {
		fs.renameSubtreePaths(oldPath, newPath)
	}

	return 0
}
