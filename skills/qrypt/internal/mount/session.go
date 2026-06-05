// Package mount glue between qrypt.SessionManager (kernel) and config.MountParams (per-mount
// driver construction). Provides a DriverFactory keyed by mount name and helpers to derive
// qrypt.SessionKey / qrypt.SessionConfig from a ResolvedMountConfig.
package mount

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"

	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	"github.com/yinzhenyu/skills/qrypt/drivers"
	factory "github.com/yinzhenyu/skills/qrypt/drivers/factory"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

// DriverFactory is a qrypt.DriverFactory that creates backend drivers from registered
// config.MountParams. Mount names are encoded in qrypt.SessionConfig.Type as "type:name".
type DriverFactory struct {
	mu            sync.RWMutex
	paramsByMount map[string]config.MountParams
}

// NewDriverFactory constructs a DriverFactory with an empty registry.
func NewDriverFactory() *DriverFactory {
	return &DriverFactory{paramsByMount: make(map[string]config.MountParams)}
}

// Register stores params for a mount. Call before Acquire.
func (f *DriverFactory) Register(mountName string, params config.MountParams) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paramsByMount[mountName] = params
}

// Unregister removes a mount from the registry. Call when no Sessions for the
// mount remain (avoid leaking memory across reload).
func (f *DriverFactory) Unregister(mountName string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.paramsByMount, mountName)
}

// CreateDriver implements qrypt.DriverFactory. Splits cfg.Type on ":" to extract
// the backend type and mount name, looks up registered params, and creates the
// adapted driver.
func (f *DriverFactory) CreateDriver(ctx context.Context, cfg qrypt.SessionConfig) (drivers.Driver, error) {
	backendType, mountName := splitTypeMount(cfg.Type)
	f.mu.RLock()
	params, ok := f.paramsByMount[mountName]
	f.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("mount %q not registered in driver factory", mountName)
	}
	drv, err := factory.NewDriverFromType(backendType, params)
	if err != nil {
		return nil, fmt.Errorf("create backend driver: %w", err)
	}
	return drv, nil
}

// SessionKeyForMount derives the qrypt.SessionKey:
//   - quark / yun139: keyed by SHA-256(cookie or auth)[:16] so mounts sharing
//     credentials share a session
//   - localfs: keyed by SHA-256(local_root)[:16] so each path gets its own
//     driver instance (no pooling collapse)
func SessionKeyForMount(rc *config.ResolvedMountConfig) qrypt.SessionKey {
	if meta, ok := drivers.GetMeta(rc.Type); ok && meta.CredentialKey != "" {
		return qrypt.SessionKey{Type: rc.Type, CredKey: hashShort(rc.Params[meta.CredentialKey])}
	}
	return qrypt.SessionKey{Type: rc.Type, CredKey: rc.Name}
}

// SessionConfigForMount builds a qrypt.SessionConfig with composite Type
// "backend:mountName" so DriverFactory.CreateDriver can look up params.
func SessionConfigForMount(mountName string, rc *config.ResolvedMountConfig) qrypt.SessionConfig {
	return qrypt.SessionConfig{Type: rc.Type + ":" + mountName}
}

func hashShort(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:16])
}

func splitTypeMount(t string) (backendType, mount string) {
	for i := 0; i < len(t); i++ {
		if t[i] == ':' {
			return t[:i], t[i+1:]
		}
	}
	return t, t
}
