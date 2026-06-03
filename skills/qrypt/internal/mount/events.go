package mount

import (
	"sync"

	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

type MountEventBus struct {
	mu   sync.RWMutex
	subs map[string]chan *protocol.Event
}

func NewMountEventBus() *MountEventBus {
	return &MountEventBus{
		subs: make(map[string]chan *protocol.Event),
	}
}

func (eb *MountEventBus) Subscribe(id string) <-chan *protocol.Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	ch := make(chan *protocol.Event, 100)
	eb.subs[id] = ch
	return ch
}

func (eb *MountEventBus) Unsubscribe(id string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	if ch, ok := eb.subs[id]; ok {
		close(ch)
		delete(eb.subs, id)
	}
}

func (eb *MountEventBus) Publish(evt *protocol.Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	for _, ch := range eb.subs {
		select {
		case ch <- evt:
		default:
		}
	}
}
