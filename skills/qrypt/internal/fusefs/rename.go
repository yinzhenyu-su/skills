//go:build !nofuse

package fusefs

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/backend"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
)

const (
	RENAME_NOREPLACE = 1
)

func (fs *QryptFS) Rename(oldPath string, newPath string) (errc int) {
	return fs.Rename3(oldPath, newPath, 0)
}

func (fs *QryptFS) Rename3(oldPath string, newPath string, flags uint32) (errc int) {
	defer func() {
		if r := recover(); r != nil {
			logging.L.Errorf("PANIC in Rename(%s->%s): %v\n", oldPath, newPath, r)
			errc = -fuse.EIO
		}
	}()
	if fs.IsShuttingDown() {
		logging.L.Warnf("[SHUTDOWN] Rejecting Rename: %s -> %s\n", oldPath, newPath)
		return -fuse.EIO
	}
	logging.L.Infof("[FUSE] Rename: oldPath=%s, newPath=%s\n", oldPath, newPath)

	oldNode, errc := fs.lookup(oldPath)
	if errc != 0 {
		return errc
	}

	if oldNode.isFolder && atomic.LoadInt32(&oldNode.uploadingChildren) > 0 {
		return -fuse.EBUSY
	}

	if flags&RENAME_NOREPLACE != 0 {
		if _, errc := fs.lookupExtended(newPath, false); errc == 0 {
			logging.L.Warnf("Rename: target exists (NOREPLACE) %s -> %s\n", oldPath, newPath)
			return -fuse.EEXIST
		}
	}

	oldParent := filepath.Dir(oldPath)
	newParent := filepath.Dir(newPath)
	newName := filepath.Base(newPath)
	isLocal := strings.HasPrefix(oldNode.fid, "local_")

	w, wOk := fs.drv.(backend.Writer)

	if !isLocal && wOk {
		encName := fs.cipher.EncryptSegment(newName)
		oldEncName := fs.cipher.EncryptSegment(oldNode.name)
		if encName != oldEncName {
			renameEntry := backend.Entry{ID: oldNode.fid, ParentID: oldNode.parentFid}
			var renameErr error
			for attempt := 0; attempt < 5; attempt++ {
				renameErr = w.Rename(context.Background(), renameEntry, encName)
				if renameErr == nil {
					break
				}
				if errors.Is(renameErr, backend.ErrDirAlreadyExists) || strings.Contains(renameErr.Error(), "conflict") {
					time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
					continue
				}
				break
			}
			if renameErr != nil {
				logging.L.Errorf("Rename: API call failed for %s -> %s: %v\n", oldPath, newPath, renameErr)
				return -fuse.EIO
			}
		}
	}

	if oldParent != newParent {
		newParentNode, errc := fs.lookup(newParent)
		if errc != 0 {
			return errc
		}

		if !isLocal && wOk {
			moveEntry := backend.Entry{ID: oldNode.fid, ParentID: oldNode.parentFid}
			var moveErr error
			for attempt := 0; attempt < 5; attempt++ {
				moveErr = w.Move(context.Background(), moveEntry, newParentNode.fid)
				if moveErr == nil {
					break
				}
				if errors.Is(moveErr, backend.ErrDirAlreadyExists) || strings.Contains(moveErr.Error(), "conflict") {
					time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
					continue
				}
				break
			}
			if moveErr != nil {
				logging.L.Errorf("Rename: move API call failed for %s -> %s: %v\n", oldPath, newPath, moveErr)
				return -fuse.EIO
			}
		}

		oldNode.mu.Lock()
		oldNode.parentFid = newParentNode.fid
		oldNode.mu.Unlock()
	}

	oldNode.mu.Lock()
	oldNode.name = newName
	oldNode.mu.Unlock()

	fs.recursiveRename(oldPath, newPath, oldNode)
	fs.persistPendingPath(oldPath, newPath, oldNode)

	return 0
}
