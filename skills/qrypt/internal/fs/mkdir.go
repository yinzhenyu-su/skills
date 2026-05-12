package fs

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/log"
	"github.com/yinzhenyu/skills/qrypt/internal/quark"
)

func (fs *QryptFS) Mkdir(path string, mode uint32) (errc int) {
	defer func() {
		if r := recover(); r != nil {
			errc = -fuse.EIO
		}
	}()
	if fs.IsShuttingDown() {
		log.L.Warnf("[SHUTDOWN] Rejecting Mkdir: %s\n", path)
		return -fuse.EIO
	}
	log.L.Infof("[FUSE] Mkdir: path=%s, mode=%o\n", path, mode)

	parentPath := filepath.Dir(path)
	name := filepath.Base(path)

	parentNode, errc := fs.lookup(parentPath)
	if errc != 0 {
		return errc
	}

	encName := fs.cipher.EncryptSegment(name)

	fid, err := fs.manageSvc.CreateDir(parentNode.fid, encName)
	if err != nil {
		if strings.Contains(err.Error(), quark.ErrDirAlreadyExists) {
			if foundFid, findErr := fs.fileSvc.FindChildByName(parentNode.fid, encName); findErr == nil {
				fid = foundFid
				if _, inDeletion := fs.activeDeletions.Load(fid); inDeletion {
					fs.activeDeletions.Delete(fid)
					if val, ok := fs.deletionsByParent.Load(parentNode.fid); ok {
						val.(*sync.Map).Delete(fid)
					}
				}
			} else {
				return -fuse.EIO
			}
		} else {
			return -fuse.EIO
		}
	}

	fs.deletingPaths.Delete(path)
	prefix := path
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	fs.deletingPaths.Range(func(key, value interface{}) bool {
		if strings.HasPrefix(key.(string), prefix) {
			fs.deletingPaths.Delete(key)
		}
		return true
	})

	fs.activeDeletions.Range(func(key, value interface{}) bool {
		state := value.(*deletionState)
		if state.path == path || strings.HasPrefix(state.path, prefix) {
			fs.activeDeletions.Delete(key)
			if val, ok := fs.deletionsByParent.Load(state.parentFid); ok {
				val.(*sync.Map).Delete(key)
			}
		}
		return true
	})

	fs.storeNode(path, &Node{
		fid:               fid,
		parentFid:         parentNode.fid,
		name:              name,
		currentPath:       path,
		isFolder:          true,
		mtime:             time.Now(),
		baseServerMtime:   time.Now().UnixMilli(),
		baseServerSize:    0,
		lastMetadataCheck: time.Now(),
		source:            "local",
	})
	fs.cacheSvc.DeleteNeg(parentNode.fid, name)
	if v, ok := fs.nodes.Load(parentPath); ok {
		fs.cacheSvc.DeleteNeg(v.(*Node).fid, name)
	}
	fs.cacheSvc.RemoveDir(parentNode.fid)

	return 0
}
