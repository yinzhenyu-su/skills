package qrypt

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
)

type SessionKey struct {
	Type    string
	CredKey string
}

type SessionConfig struct {
	Type   string
	Cookie string
	Auth   string
	RootID string
}

type DriverFactory interface {
	CreateDriver(ctx context.Context, cfg SessionConfig) (Driver, error)
}

type Session struct {
	Drv      Driver
	RefCount int32
}

type SessionManager interface {
	Acquire(ctx context.Context, key SessionKey, cfg SessionConfig) (*Session, error)
	Release(ctx context.Context, key SessionKey)
}

type sessionManager struct {
	mu       sync.Mutex
	sessions map[SessionKey]*Session
	factory  DriverFactory
}

func NewSessionManager(factory DriverFactory) SessionManager {
	return &sessionManager{
		sessions: make(map[SessionKey]*Session),
		factory:  factory,
	}
}

func (sm *sessionManager) Acquire(ctx context.Context, key SessionKey, cfg SessionConfig) (*Session, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if s, ok := sm.sessions[key]; ok {
		s.RefCount++
		return s, nil
	}

	drv, err := sm.factory.CreateDriver(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create driver: %w", err)
	}
	if err := drv.Init(ctx); err != nil {
		return nil, fmt.Errorf("init driver: %w", err)
	}

	s := &Session{Drv: drv, RefCount: 1}
	sm.sessions[key] = s
	return s, nil
}

func (sm *sessionManager) Release(ctx context.Context, key SessionKey) {
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

func CredKeyForCookie(cookie string) string {
	h := sha256.Sum256([]byte(cookie))
	return fmt.Sprintf("%x", h[:16])
}

func CredKeyForAuth(auth string) string {
	h := sha256.Sum256([]byte(auth))
	return fmt.Sprintf("%x", h[:16])
}
