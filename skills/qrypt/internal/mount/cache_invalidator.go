package mount

import (
	"encoding/json"
	"path/filepath"

	"github.com/yinzhenyu/skills/qrypt/internal/logging"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

type CacheInvalidator struct {
	manager *MountManager
	subID   string
}

func NewCacheInvalidator(mgr *MountManager, mb *MountEventBus) *CacheInvalidator {
	ci := &CacheInvalidator{
		manager: mgr,
		subID:   "cache_invalidator",
	}
	go ci.listen(mb)
	return ci
}

func (ci *CacheInvalidator) listen(mb *MountEventBus) {
	ch := mb.Subscribe(ci.subID)
	defer mb.Unsubscribe(ci.subID)

	for evt := range ch {
		if evt.Type != protocol.EventSyncCompleted {
			continue
		}
		data, _ := json.Marshal(evt.Data)
		var progress protocol.PushProgressData
		if err := json.Unmarshal(data, &progress); err != nil {
			continue
		}
		if progress.Mount != "" {
			ci.invalidateMount(progress.Mount, progress.File)
		} else {
			ci.invalidateAll(progress.File)
		}
	}
}

func (ci *CacheInvalidator) invalidateMount(mountName, remotePath string) {
	dir := filepath.Dir(remotePath)
	ci.manager.ForEachRunningMount(func(name string, inst *MountInstance) {
		if name != mountName {
			return
		}
		if inst.State != protocol.MountStateMounted || inst.Backend == nil {
			return
		}
		vfs := inst.Backend.VFS()
		if vfs == nil {
			return
		}
		vfs.InvalidateDirCache(dir)
		logging.L.Debugf("CacheInvalidator: evicted %q from mount %q\n", dir, name)
	})
}

func (ci *CacheInvalidator) invalidateAll(remotePath string) {
	dir := filepath.Dir(remotePath)
	ci.manager.ForEachRunningMount(func(name string, inst *MountInstance) {
		if inst.State != protocol.MountStateMounted || inst.Backend == nil {
			return
		}
		vfs := inst.Backend.VFS()
		if vfs == nil {
			return
		}
		vfs.InvalidateDirCache(dir)
		logging.L.Debugf("CacheInvalidator: evicted %q from mount %q\n", dir, name)
	})
}
