package mobile

import (
	"context"
	"fmt"

	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	"github.com/yinzhenyu/skills/qrypt/internal/backend"
	"github.com/yinzhenyu/skills/qrypt/internal/backend/quark"
	"github.com/yinzhenyu/skills/qrypt/internal/backend/yun139"
	"github.com/yinzhenyu/skills/qrypt/internal/coreadapter"
)

// mobileDriverFactory creates backend drivers on demand, looking up credentials from the
// CredentialStore by key derived from SessionConfig.Type. Supported types: "quark", "yun139".
//
// Credential key convention:
//
//	cookie_<mountName>   for quark   (full Quark cookie string)
//	auth_<mountName>     for yun139  (Authorization header value)
//
// SessionConfig fields:
//
//	Type   = backend type ("quark" | "yun139") + optional ":mountName" suffix.
//	         e.g. "quark:photos" → looks up "cookie_photos".
//	         Plain "quark" → looks up "cookie_quark".
//	RootID = backend-specific root identifier (rootPath for quark, rootID for yun139).
type mobileDriverFactory struct {
	creds qrypt.CredentialStore
}

func newMobileDriverFactory(creds qrypt.CredentialStore) qrypt.DriverFactory {
	return &mobileDriverFactory{creds: creds}
}

func (f *mobileDriverFactory) CreateDriver(ctx context.Context, cfg qrypt.SessionConfig) (qrypt.Driver, error) {
	backendType, mountName := splitTypeMount(cfg.Type)

	var drv backend.Driver
	switch backendType {
	case "quark":
		cookie, err := f.creds.Get("cookie_" + mountName)
		if err != nil {
			return nil, fmt.Errorf("get quark cookie for mount %q: %w", mountName, err)
		}
		drv = quark.NewDriver(cookie, cfg.RootID)
	case "yun139":
		auth, err := f.creds.Get("auth_" + mountName)
		if err != nil {
			return nil, fmt.Errorf("get yun139 auth for mount %q: %w", mountName, err)
		}
		drv = yun139.NewDriver(auth, cfg.RootID)
	default:
		return nil, fmt.Errorf("unsupported backend type: %q (supported: quark, yun139)", backendType)
	}

	return coreadapter.NewDriverAdapter(drv), nil
}

func splitTypeMount(t string) (backendType, mount string) {
	for i := 0; i < len(t); i++ {
		if t[i] == ':' {
			return t[:i], t[i+1:]
		}
	}
	return t, t
}
