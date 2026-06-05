package platform

import (
	"fmt"
	"path/filepath"

	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

type DesktopDirResolver struct{}

func (DesktopDirResolver) CacheDir() string {
	return filepath.Join(config.WorkDir(), "cache")
}

func (DesktopDirResolver) DataDir() string {
	return config.WorkDir()
}

func (DesktopDirResolver) ConfigDir() string {
	return config.WorkDir()
}

type TomlCredentialStore struct {
	cfg *config.Config
}

func (s *TomlCredentialStore) Get(key string) (string, error) {
	for _, m := range s.cfg.Mounts {
		meta, ok := drivers.GetMeta(m.Type)
		if !ok || meta.CredentialKey == "" {
			continue
		}
		prefix := credKeyPrefix(m.Type)
		if prefix+m.Name == key {
			return m.Params[meta.CredentialKey], nil
		}
	}
	return "", qrypt.NewErrorf(qrypt.ErrNotFound, "credential not found: %s", key)
}

func (s *TomlCredentialStore) Set(_, _ string) error {
	return fmt.Errorf("toml credential store is read-only; edit qrypt.toml directly")
}

func (s *TomlCredentialStore) Delete(_ string) error {
	return fmt.Errorf("toml credential store is read-only; edit qrypt.toml directly")
}

// credKeyPrefix returns the CredentialStore key prefix for the given backend type.
func credKeyPrefix(backendType string) string {
	switch backendType {
	case "quark":
		return "cookie_"
	case "yun139":
		return "auth_"
	default:
		return "cred_"
	}
}
