package daemon

import (
	"sync"
	"time"

	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

type ProgressEntry struct {
	TaskID    string
	Mount     string
	Direction string // "push" or "pull"
	File      string
	Bytes     int64
	Total     int64
	State     string
	Error     string
	UpdatedAt time.Time
}

// ProgressHub collects transfer progress from all sources (daemon push/pull, VFS).
// Publishes live events via EventManager and tracks active transfers for status queries.
type ProgressHub struct {
	eventMgr *EventManager

	mu     sync.RWMutex
	active map[string]*ProgressEntry // taskID → entry
}

func NewProgressHub(em *EventManager) *ProgressHub {
	return &ProgressHub{
		eventMgr: em,
		active:   make(map[string]*ProgressEntry),
	}
}

// Publish records a progress update and pushes an event.
func (ph *ProgressHub) Publish(entry *ProgressEntry) {
	entry.UpdatedAt = time.Now()

	ph.mu.Lock()
	if entry.State == "completed" || entry.State == "failed" {
		delete(ph.active, entry.TaskID)
	} else {
		ph.active[entry.TaskID] = entry
	}
	ph.mu.Unlock()

	evtType := protocol.EventSyncProgress
	switch entry.State {
	case "completed":
		evtType = protocol.EventSyncCompleted
	case "failed":
		evtType = protocol.EventSyncFailed
	}

	ph.eventMgr.Publish(&protocol.Event{
		Type:      evtType,
		Timestamp: time.Now().UnixMilli(),
		Data: protocol.PushProgressData{
			TaskID: entry.TaskID,
			File:   entry.File,
			Bytes:  entry.Bytes,
			Total:  entry.Total,
			State:  entry.State,
			Error:  entry.Error,
		},
	})
}

// Active returns a snapshot of all active transfers.
func (ph *ProgressHub) Active() []ProgressEntry {
	ph.mu.RLock()
	defer ph.mu.RUnlock()

	out := make([]ProgressEntry, 0, len(ph.active))
	for _, e := range ph.active {
		out = append(out, *e)
	}
	return out
}
