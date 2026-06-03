//go:build !nofuse

package fusefs

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/backend"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
)

func (fs *QryptFS) Mkdir(path string, mode uint32) (errc int) {
	defer func() {
		if r := recover(); r != nil {
			errc = -fuse.EIO
		}
	}()
	if fs.IsShuttingDown() {
		logging.L.Warnf("[SHUTDOWN] Rejecting Mkdir: %s\n", path)
		return -fuse.EIO
	}
	logging.L.Infof("[FUSE] Mkdir: path=%s, mode=%o\n", path, mode)

	parentPath := filepath.Dir(path)
	name := filepath.Base(path)

	parentNode, errc := fs.lookup(parentPath)
	if errc != 0 {
		return errc
	}

	encName := fs.cipher.EncryptSegment(name)

	w, ok := fs.drv.(backend.Writer)
	if !ok {
		logging.L.Errorf("Mkdir: driver does not support write operations\n")
		return -fuse.EIO
	}

	var fid string
	entry, err := w.Mkdir(context.Background(), parentNode.fid, encName)
	if err != nil {
		if errors.Is(err, backend.ErrDirAlreadyExists) {
			entries, listErr := fs.drv.List(context.Background(), parentNode.fid)
			if listErr != nil {
				return -fuse.EIO
			}
			for _, e := range entries {
				if e.Name == encName && e.IsDir {
					fid = e.ID
					break
				}
			}
			if fid == "" {
				return -fuse.EIO
			}
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
		fid = entry.ID
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

	return 0
}
