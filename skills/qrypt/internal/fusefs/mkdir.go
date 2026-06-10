//go:build !nofuse

package fusefs

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/winfsp/cgofuse/fuse"
	"github.com/yinzhenyu/skills/qrypt/internal/logging"
)

func (fs *QryptFS) Mkdir(path string, mode uint32) (errc int) {
	start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			errc = -fuse.EIO
		}
		logging.L.Debugf("[TIMER] Mkdir(%s): took %v\n", path, time.Since(start))
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

	// Create the directory locally with a local_ fid, same as files.
	// The remote directory is created asynchronously during the first
	// file upload inside it (via ensureParentDirExists in syncFile).
	// This avoids blocking the FUSE callback on a slow remote API call.
	fid := "local_" + name + "_" + fmt.Sprint(time.Now().UnixNano())

	fs.deletingPaths.Delete(path)
	prefix := path + "/"
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
		baseServerMtime:   0,
		baseServerSize:    0,
		lastMetadataCheck: time.Now(),
		source:            "local",
	})

	return 0
}
