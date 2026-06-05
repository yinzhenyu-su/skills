package mobile

import (
	"context"
	"fmt"

	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	"github.com/yinzhenyu/skills/qrypt/drivers"

	// Register drivers available on mobile.
	_ "github.com/yinzhenyu/skills/qrypt/drivers/quark"
	_ "github.com/yinzhenyu/skills/qrypt/drivers/yun139"
	// localfs intentionally omitted on mobile
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

func (f *mobileDriverFactory) CreateDriver(ctx context.Context, cfg qrypt.SessionConfig) (drivers.Driver, error) {
	backendType, mountName := splitTypeMount(cfg.Type)

	credKey := credKeyPrefix(backendType) + mountName
	cred, err := f.creds.Get(credKey)
	if err != nil {
		return nil, fmt.Errorf("get credential for %s mount %q: %w", backendType, mountName, err)
	}

	params := drivers.Params{
		credParamName(backendType): cred,
		"root_id":                  cfg.RootID,
	}
	return drivers.New(backendType, params)
}

func splitTypeMount(t string) (backendType, mount string) {
	for i := 0; i < len(t); i++ {
		if t[i] == ':' {
			return t[:i], t[i+1:]
		}
	}
	return t, t
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

// credParamName returns the drivers.Params key name for the credential value.
func credParamName(backendType string) string {
	switch backendType {
	case "quark":
		return "cookie"
	case "yun139":
		return "authorization"
	default:
		return "credential"
	}
}
