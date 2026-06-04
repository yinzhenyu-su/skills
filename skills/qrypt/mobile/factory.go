package mobile

import (
	"fmt"

	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
)

// Config holds construction-time parameters supplied by the Android/iOS host.
// All fields are gomobile-compatible primitives.
type Config struct {
	// Password is the rclone-crypt password (plaintext). Required.
	Password string
	// Salt is the rclone-crypt password2 (salt). Required.
	Salt string
	// NameEncoding is one of "base32", "base64" (default: "base32").
	NameEncoding string
	// FilenameEncryption is one of "standard", "obfuscate", "off" (default: "standard").
	FilenameEncryption string

	// CacheDir / DataDir / ConfigDir — pass from Android Context.
	CacheDir  string
	DataDir   string
	ConfigDir string

	// RateLimitBPS — 0 = unlimited.
	RateLimitBPS int64
	// NumWorkers — default 3.
	NumWorkers int
}

// NewQryptAPI is the primary entry point for Android/iOS.
//
// Wiring:
//
//	cipher        ← qrypt.NewRcloneCipher(Password, Salt, ...)
//	creds         ← in-memory MemoryCredentialStore (host must Set() before use)
//	dirs          ← MobileDirs from cfg
//	DriverFactory ← mobileDriverFactory (quark / yun139)
//
// For Keystore-backed credentials, use NewQryptAPIWithProvider.
func NewQryptAPI(cfg *Config) (*MobileAPI, error) {
	creds := NewMemoryCredentialStore()
	return newQryptAPI(cfg, creds)
}

// NewQryptAPIWithProvider is the variant that takes a host-supplied CredentialProvider
// (typically Android Keystore). The provider is consulted on every driver creation.
func NewQryptAPIWithProvider(cfg *Config, provider CredentialProvider) (*MobileAPI, error) {
	if provider == nil {
		return nil, fmt.Errorf("credential provider is required")
	}
	return newQryptAPI(cfg, providerStore{inner: provider})
}

// SetCredential is a convenience for hosts using the default MemoryCredentialStore.
// Returns an error if the store is not a MemoryCredentialStore (e.g. provider-backed).
func (m *MobileAPI) SetCredential(key, value string) error {
	if mem, ok := m.creds.(*MemoryCredentialStore); ok {
		return mem.Set(key, value)
	}
	return fmt.Errorf("credential store is not in-memory; set via CredentialProvider")
}

// DeleteCredential is the inverse of SetCredential.
func (m *MobileAPI) DeleteCredential(key string) error {
	if mem, ok := m.creds.(*MemoryCredentialStore); ok {
		return mem.Delete(key)
	}
	return fmt.Errorf("credential store is not in-memory; delete via CredentialProvider")
}

func newQryptAPI(cfg *Config, creds qrypt.CredentialStore) (*MobileAPI, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if cfg.Password == "" || cfg.Salt == "" {
		return nil, fmt.Errorf("password and salt are required")
	}

	encoding := cfg.NameEncoding
	if encoding == "" {
		encoding = "base32"
	}
	filenameEnc := cfg.FilenameEncryption
	if filenameEnc == "" {
		filenameEnc = "standard"
	}

	rc, err := qrypt.NewRcloneCipher(cfg.Password, cfg.Salt, encoding, filenameEnc)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	dirs := MobileDirs{
		Cache:  cfg.CacheDir,
		Data:   cfg.DataDir,
		Config: cfg.ConfigDir,
	}

	api, err := qrypt.NewFileAPI(qrypt.Options{
		Cipher:        rc,
		Dirs:          dirs,
		Creds:         creds,
		DriverFactory: newMobileDriverFactory(creds),
		RateLimitBPS:  cfg.RateLimitBPS,
		NumWorkers:    cfg.NumWorkers,
	})
	if err != nil {
		return nil, fmt.Errorf("create FileAPI: %w", err)
	}

	return &MobileAPI{
		fileAPI: api,
		creds:   creds,
		streams: newStreamRegistry(),
	}, nil
}
