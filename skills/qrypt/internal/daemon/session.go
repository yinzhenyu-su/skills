package daemon

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"

	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
	factory "github.com/yinzhenyu/skills/qrypt/internal/drive/factory"
)

// SessionKey uniquely identifies a driver session by type and credential hash.
type SessionKey struct {
	Type    string
	CredKey string // sha256 of auth-sensitive params
}

// Session wraps a drive.Driver with reference counting.
type Session struct {
	Drv      drive.Driver
	RefCount int32
}

// SessionManager manages driver sessions with shared reference counting.
// VFS mounts and CLI operations borrow from the same pool.
type SessionManager struct {
	mu       sync.Mutex
	sessions map[SessionKey]*Session
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[SessionKey]*Session),
	}
}

// Acquire gets or creates a session for the given key.
// Returns nil, nil for drivers that don't need sessions (localfs).
func (sm *SessionManager) Acquire(ctx context.Context, key SessionKey, params config.MountParams) (*Session, error) {
	if key.Type == "localfs" {
		drv, err := factory.NewDriverFromType(key.Type, params)
		if err != nil {
			return nil, err
		}
		return &Session{Drv: drv}, nil
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	if s, ok := sm.sessions[key]; ok {
		s.RefCount++
		return s, nil
	}

	drv, err := factory.NewDriverFromType(key.Type, params)
	if err != nil {
		return nil, fmt.Errorf("driver init: %w", err)
	}
	if err := drv.Init(ctx); err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	s := &Session{Drv: drv, RefCount: 1}
	sm.sessions[key] = s
	return s, nil
}

// Release decrements the session refcount; drops the driver when it reaches 0.
func (sm *SessionManager) Release(ctx context.Context, key SessionKey) {
	if key.Type == "localfs" {
		return
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	s, ok := sm.sessions[key]
	if !ok {
		return
	}
	s.RefCount--
	if s.RefCount <= 0 {
		s.Drv.Drop(ctx)
		delete(sm.sessions, key)
	}
}

// SessionKeyForMount derives a SessionKey from a resolved mount config.
func SessionKeyForMount(rc *config.ResolvedMountConfig) (SessionKey, bool) {
	switch rc.Type {
	case "localfs":
		return SessionKey{Type: "localfs"}, false
	case "quark":
		h := sha256.Sum256([]byte(rc.Params.Cookie))
		return SessionKey{Type: "quark", CredKey: fmt.Sprintf("%x", h[:16])}, true
	case "yun139":
		h := sha256.Sum256([]byte(rc.Params.Authorization))
		return SessionKey{Type: "yun139", CredKey: fmt.Sprintf("%x", h[:16])}, true
	default:
		return SessionKey{}, false
	}
}
