package config

import (
	"fmt"
	"slices"
	"strings"

	"github.com/yinzhenyu/skills/qrypt/drivers"
)

type ValidationCheck struct {
	Field   string `json:"field"`
	Status  string `json:"status"` // "ok" | "warn" | "error"
	Message string `json:"message"`
}

type ValidationResult struct {
	Valid    bool              `json:"valid"`
	FilePath string            `json:"file_path,omitempty"`
	Checks   []ValidationCheck `json:"checks"`
}

func (r *ValidationResult) ChecksMap() map[string]string {
	m := make(map[string]string, len(r.Checks))
	for _, c := range r.Checks {
		m[c.Field] = c.Status
	}
	return m
}

func ValidateConfig(cfg *Config) *ValidationResult {
	r := &ValidationResult{}

	if len(cfg.Mounts) == 0 {
		r.addCheck("mounts", "error", "no mount instances configured — add [[mounts]] to configuration")
	} else {
		defaultCount := 0
		for _, m := range cfg.Mounts {
			if m.Default {
				defaultCount++
			}
		}
		if defaultCount > 1 {
			r.addCheck("mounts", "error", fmt.Sprintf("%d mounts have default = true — at most one allowed", defaultCount))
		}

		names := make(map[string]bool)
		for i, m := range cfg.Mounts {
			prefix := fmt.Sprintf("mounts[%d]", i)

			// Name
			if m.Name == "" {
				r.addCheck(prefix+".name", "error", "mount name is required")
			} else if !validMountName.MatchString(m.Name) {
				r.addCheck(prefix+".name", "error", "mount name must match [a-z0-9-]{1,32}")
			} else if names[m.Name] {
				r.addCheck(prefix+".name", "error", fmt.Sprintf("duplicate mount name: %q", m.Name))
			} else {
				r.addCheck(prefix+".name", "ok", m.Name)
			}
			names[m.Name] = true

			// Type
			status, msg := validateDriveType(m.Type)
			r.addCheck(prefix+".type", status, msg)

			// Mount point
			if m.MountPoint != "" {
				r.addCheck(prefix+".mount_point", "ok", m.MountPoint)
			} else {
				r.addCheck(prefix+".mount_point", "error", "mount_point is required")
			}

			// Driver-specific params
			switch m.Type {
			case "quark":
				if m.Params.Cookie == "" {
					r.addCheck(prefix+".params.cookie", "error", "cookie is required for quark driver")
				} else {
					r.addCheck(prefix+".params.cookie", "ok", "set")
				}
			case "yun139":
				if m.Params.Authorization == "" {
					r.addCheck(prefix+".params.authorization", "error", "authorization is required for yun139 driver")
				} else {
					r.addCheck(prefix+".params.authorization", "ok", "set")
				}
			case "localfs":
				if m.Params.LocalRoot == "" && m.Params.RootPath == "" {
					r.addCheck(prefix+".params.local_root", "error", "local_root is required for localfs driver")
				} else {
					r.addCheck(prefix+".params.local_root", "ok", "set")
				}
			}

			// Encryption
			hasEnc := m.Encryption != nil && m.Encryption.Password != ""
			hasDef := cfg.Defaults.Encryption.Password != ""
			if !hasEnc && !hasDef {
				r.addCheck(prefix+".encryption.password", "warn", "no encryption password set (use mount-level or defaults)")
			}
		}
	}

	// Defaults validation.
	if _, err := ParseSize(cfg.Defaults.Cache.MaxSize); err != nil {
		r.addCheck("defaults.cache.max_size", "error", fmt.Sprintf("unable to parse: %s", err))
	} else {
		r.addCheck("defaults.cache.max_size", "ok", cfg.Defaults.Cache.MaxSize)
	}

	if cfg.Defaults.Sync.DirCacheTTL != "" {
		if _, err := ParseDuration(cfg.Defaults.Sync.DirCacheTTL); err != nil {
			r.addCheck("defaults.sync.dir_cache_ttl", "error", fmt.Sprintf("unable to parse: %s", err))
		} else {
			r.addCheck("defaults.sync.dir_cache_ttl", "ok", cfg.Defaults.Sync.DirCacheTTL)
		}
	}

	if cfg.Defaults.Sync.MaxRetries < 0 {
		r.addCheck("defaults.sync.max_retries", "error", "must not be negative")
	} else {
		r.addCheck("defaults.sync.max_retries", "ok", fmt.Sprintf("%d", cfg.Defaults.Sync.MaxRetries))
	}
	if cfg.Defaults.Sync.ConcurrentUploads <= 0 {
		r.addCheck("defaults.sync.concurrent_uploads", "error", "must be greater than 0")
	} else {
		r.addCheck("defaults.sync.concurrent_uploads", "ok", fmt.Sprintf("%d", cfg.Defaults.Sync.ConcurrentUploads))
	}

	// Per-mount cache/sync overrides are optional — inherit from defaults.

	// Log level.
	if cfg.Log.Level != "" {
		if !isValidLogLevel(cfg.Log.Level) {
			r.addCheck("log.level", "error", fmt.Sprintf("unknown level: %s", cfg.Log.Level))
		} else {
			r.addCheck("log.level", "ok", cfg.Log.Level)
		}
	}

	r.Valid = true
	for _, c := range r.Checks {
		if c.Status == "error" {
			r.Valid = false
		}
	}
	return r
}

func ValidateConfigFile(path string) *ValidationResult {
	_, vr, err := LoadConfig(path)
	if err != nil {
		return &ValidationResult{
			Valid:    false,
			FilePath: path,
			Checks: []ValidationCheck{
				{Field: "config.file", Status: "error", Message: err.Error()},
			},
		}
	}
	vr.FilePath = path
	return vr
}

func (r *ValidationResult) addCheck(field, status, message string) {
	r.Checks = append(r.Checks, ValidationCheck{Field: field, Status: status, Message: message})
}

func validateDriveType(t string) (string, string) {
	supported := drivers.SupportedTypes()
	if t == "" {
		return "error", fmt.Sprintf("drive type not set (must be %s)", strings.Join(supported, ", "))
	}
	if slices.Contains(supported, t) {
		return "ok", t
	}
	return "error", fmt.Sprintf("unknown drive type: %s (supported: %s)", t, strings.Join(supported, ", "))
}

func validateRequired(field, value string, required bool) (string, string) {
	if value == "" {
		if required {
			return "error", "not set (required)"
		}
		return "warn", "not set (optional)"
	}
	return "ok", "set"
}

func validateOptional(field, value string) (string, string) {
	return validateRequired(field, value, false)
}

func validateOption[T any](ptr *T, label string) (string, string) {
	if ptr == nil {
		return "warn", label + " section not configured"
	}
	return "ok", label + " configured"
}

func isValidLogLevel(s string) bool {
	switch s {
	case "debug", "info", "warn", "warning", "error", "off", "none":
		return true
	}
	return false
}
