package mobile

import (
	"sync"

	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
)

// MemoryCredentialStore is an in-process credential map. Android should NOT keep
// raw secrets in memory long-term — instead, fetch from Keystore on demand and
// call Set() right before each session is created, then call Delete() after.
//
// For a Keystore-backed implementation, expose a Go interface that Android
// implements via gomobile reverse-binding (see CredentialProvider below).
type MemoryCredentialStore struct {
	mu     sync.RWMutex
	values map[string]string
}

func NewMemoryCredentialStore() *MemoryCredentialStore {
	return &MemoryCredentialStore{values: make(map[string]string)}
}

func (s *MemoryCredentialStore) Get(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.values[key]
	if !ok {
		return "", qrypt.NewErrorf(qrypt.ErrNotFound, "credential not found: %s", key)
	}
	return v, nil
}

func (s *MemoryCredentialStore) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
	return nil
}

func (s *MemoryCredentialStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, key)
	return nil
}

// CredentialProvider is the gomobile-friendly interface Android can implement
// to back the credential store with Keystore / iOS Keychain. Pass an impl into
// NewQryptAPIWithProvider.
//
// Kotlin example:
//
//	class KeystoreCreds : CredentialProvider {
//	    override fun get(key: String): String { /* read Keystore */ }
//	    override fun set(key: String, value: String) { /* write Keystore */ }
//	    override fun delete(key: String) { /* remove Keystore */ }
//	}
type CredentialProvider interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
}

type providerStore struct{ inner CredentialProvider }

func (p providerStore) Get(key string) (string, error)        { return p.inner.Get(key) }
func (p providerStore) Set(key, value string) error           { return p.inner.Set(key, value) }
func (p providerStore) Delete(key string) error               { return p.inner.Delete(key) }
