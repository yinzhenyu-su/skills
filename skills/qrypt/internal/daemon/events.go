package daemon

import (
	"sync"

	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

// EventManager manages event subscriptions and broadcasting.
type EventManager struct {
	mu      sync.RWMutex
	subs    map[string]chan *protocol.Event
}

// NewEventManager creates a new event manager.
func NewEventManager() *EventManager {
	return &EventManager{
		subs: make(map[string]chan *protocol.Event),
	}
}

// Subscribe creates a new event subscription.
func (em *EventManager) Subscribe(id string) <-chan *protocol.Event {
	em.mu.Lock()
	defer em.mu.Unlock()
	ch := make(chan *protocol.Event, 100)
	em.subs[id] = ch
	return ch
}

// Unsubscribe removes an event subscription.
func (em *EventManager) Unsubscribe(id string) {
	em.mu.Lock()
	defer em.mu.Unlock()
	if ch, ok := em.subs[id]; ok {
		close(ch)
		delete(em.subs, id)
	}
}

// Publish broadcasts an event to all subscribers.
func (em *EventManager) Publish(evt *protocol.Event) {
	em.mu.RLock()
	defer em.mu.RUnlock()
	for _, ch := range em.subs {
		select {
		case ch <- evt:
		default:
			// Drop event if subscriber is slow
		}
	}
}
