package daemon

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/yinzhenyu/skills/qrypt/internal/log"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

// CacheInvalidator listens for transfer completion events and evicts stale
// VFS directory cache entries so subsequent reads fetch fresh data from the remote.
type CacheInvalidator struct {
	manager *MountManager
	subID   string
}

// NewCacheInvalidator creates a CacheInvalidator that subscribes to the given EventManager.
func NewCacheInvalidator(mgr *MountManager, em *EventManager) *CacheInvalidator {
	ci := &CacheInvalidator{
		manager: mgr,
		subID:   "cache_invalidator",
	}
	go ci.listen(em)
	return ci
}

func (ci *CacheInvalidator) listen(em *EventManager) {
	ch := em.Subscribe(ci.subID)
	defer em.Unsubscribe(ci.subID)

	for evt := range ch {
		if evt.Type != protocol.EventSyncCompleted {
			continue
		}
		data, _ := json.Marshal(evt.Data)
		var progress protocol.PushProgressData
		if err := json.Unmarshal(data, &progress); err != nil {
			continue
		}
		// progress.Mount is not in PushProgressData currently.
		// For now, invalidate on all mounts — conservative but correct.
		ci.invalidateAll(context.Background(), progress.File)
	}
}

func (ci *CacheInvalidator) invalidateAll(ctx context.Context, remotePath string) {
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
		log.L.Debugf("CacheInvalidator: evicted %q from mount %q\n", dir, name)
	})
}
