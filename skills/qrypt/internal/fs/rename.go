package fs

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/log"
	"github.com/yinzhenyu/skills/qrypt/internal/quark"
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

	if oldParent != newParent {
		newParentNode, errc := fs.lookup(newParent)
		if errc != 0 {
			return errc
		}

		if !isLocal {
			var moveErr error
			for attempt := 0; attempt < 5; attempt++ {
				moveErr = fs.manageSvc.Move([]string{oldNode.fid}, newParentNode.fid, oldNode.parentFid)
				if moveErr == nil {
					break
				}
				if strings.Contains(moveErr.Error(), quark.ErrDirAlreadyExists) || strings.Contains(moveErr.Error(), "conflict") {
					time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
					fs.cacheSvc.RemoveDir(newParentNode.fid)
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

		oldParentNode, errc := fs.lookup(oldParent)
		if errc == 0 {
			fs.cacheSvc.RemoveDir(oldParentNode.fid)
		}
		fs.cacheSvc.RemoveDir(newParentNode.fid)
	}

	if !isLocal {
		encName := fs.cipher.EncryptSegment(newName)
		oldEncName := fs.cipher.EncryptSegment(oldNode.name)
		if encName != oldEncName {
			var renameErr error
			for attempt := 0; attempt < 5; attempt++ {
				renameErr = fs.manageSvc.Rename(oldNode.fid, encName)
				if renameErr == nil {
					break
				}
				if strings.Contains(renameErr.Error(), quark.ErrDirAlreadyExists) || strings.Contains(renameErr.Error(), "conflict") {
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

	parentNode, errc := fs.lookup(newParent)
	if errc == 0 {
		fs.cacheSvc.DeleteNeg(parentNode.fid, newName)
		fs.cacheSvc.RemoveDir(parentNode.fid)
	}
	if oldParent != newParent {
		oldParentNode, errc := fs.lookup(oldParent)
		if errc == 0 {
			fs.cacheSvc.RemoveDir(oldParentNode.fid)
		}
	}

	return 0
}
