package qrypt

import (
	"sync"
	"time"
)

type EventType string

const (
	EventSyncProgress  EventType = "sync_progress"
	EventSyncCompleted EventType = "sync_completed"
	EventSyncFailed    EventType = "sync_failed"
)

type Event struct {
	Type      EventType
	Mount     string
	Timestamp int64
	Progress  *ProgressEntry
}

type EventBus interface {
	Subscribe(id string) <-chan *Event
	Unsubscribe(id string)
	Publish(evt *Event)
}

type eventBus struct {
	mu   sync.RWMutex
	subs map[string]chan *Event
}

func NewEventBus() EventBus {
	return &eventBus{
		subs: make(map[string]chan *Event),
	}
}

func (eb *eventBus) Subscribe(id string) <-chan *Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	ch := make(chan *Event, 100)
	eb.subs[id] = ch
	return ch
}

func (eb *eventBus) Unsubscribe(id string) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	if ch, ok := eb.subs[id]; ok {
		close(ch)
		delete(eb.subs, id)
	}
}

func (eb *eventBus) Publish(evt *Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()
	for _, ch := range eb.subs {
		select {
		case ch <- evt:
		default:
		}
	}
}

type ProgressEntry struct {
	TaskID    string
	Mount     string
	Direction string
	File      string
	Bytes     int64
	Total     int64
	State     string
	Error     string
	UpdatedAt time.Time
}

type ProgressHub interface {
	Publish(entry *ProgressEntry)
	Active() []ProgressEntry
}

type progressHub struct {
	eventBus EventBus

	mu     sync.RWMutex
	active map[string]*ProgressEntry
}

func NewProgressHub(eb EventBus) ProgressHub {
	return &progressHub{
		eventBus: eb,
		active:   make(map[string]*ProgressEntry),
	}
}

func (ph *progressHub) Publish(entry *ProgressEntry) {
	entry.UpdatedAt = time.Now()

	ph.mu.Lock()
	if entry.State == "completed" || entry.State == "failed" {
		delete(ph.active, entry.TaskID)
	} else {
		ph.active[entry.TaskID] = entry
	}
	ph.mu.Unlock()

	evtType := EventSyncProgress
	switch entry.State {
	case "completed":
		evtType = EventSyncCompleted
	case "failed":
		evtType = EventSyncFailed
	}

	ph.eventBus.Publish(&Event{
		Type:      evtType,
		Mount:     entry.Mount,
		Timestamp: time.Now().UnixMilli(),
		Progress:  entry,
	})
}

func (ph *progressHub) Active() []ProgressEntry {
	ph.mu.RLock()
	defer ph.mu.RUnlock()

	out := make([]ProgressEntry, 0, len(ph.active))
	for _, e := range ph.active {
		out = append(out, *e)
	}
	return out
}
