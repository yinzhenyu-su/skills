package qrypt

import "path/filepath"

type MountLister interface {
	ForEachRunningMount(fn func(name string) bool)
}

type CacheInvalidatorHooks interface {
	InvalidateDirCache(mountName, dirPath string) error
}

type CacheInvalidator struct {
	hooks CacheInvalidatorHooks
	subID string
	bus   EventBus
}

func NewCacheInvalidator(hooks CacheInvalidatorHooks, bus EventBus) *CacheInvalidator {
	ci := &CacheInvalidator{
		hooks: hooks,
		subID: "cache_invalidator",
		bus:   bus,
	}
	ci.start()
	return ci
}

func (ci *CacheInvalidator) start() {
	ch := ci.bus.Subscribe(ci.subID)
	go ci.listen(ch)
}

func (ci *CacheInvalidator) listen(ch <-chan *Event) {
	defer ci.bus.Unsubscribe(ci.subID)

	for evt := range ch {
		if evt.Type != EventSyncCompleted {
			continue
		}
		if evt.Progress == nil {
			continue
		}
		if evt.Progress.Mount != "" && evt.Progress.File != "" {
			dir := filepath.Dir(evt.Progress.File)
			ci.hooks.InvalidateDirCache(evt.Progress.Mount, dir)
		}
	}
}
