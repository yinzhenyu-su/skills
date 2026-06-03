package platform

import (
	"fmt"
	"path/filepath"

	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
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
		if "cookie_"+m.Name == key {
			return m.Params.Cookie, nil
		}
		if "auth_"+m.Name == key {
			return m.Params.Authorization, nil
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
