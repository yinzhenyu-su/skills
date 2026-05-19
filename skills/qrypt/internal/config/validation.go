package config

import "fmt"

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

func ValidateConfig(cfg *Config) *ValidationResult {
	r := &ValidationResult{}
	status, msg := validateDriveType(cfg.Drive.Type)
	r.addCheck("drive.type", status, msg)

	status, msg = validateRequired("encryption.password", cfg.Encryption.Password, true)
	r.addCheck("encryption.password", status, msg)

	status, msg = validateOptional("encryption.salt", cfg.Encryption.Salt)
	r.addCheck("encryption.salt", status, msg)

	status, msg = validateRequired("mount.point", cfg.Mount.Point, true)
	r.addCheck("mount.point", status, msg)

	// Validate drive-specific settings.
	switch cfg.Drive.Type {
	case "quark":
		status, msg = validateOption(cfg.Drive.Quark, "quark")
		r.addCheck("drive.quark", status, msg)
		if cfg.Drive.Quark != nil {
			status, msg = validateRequired("drive.quark.cookie", cfg.Drive.Quark.Cookie, true)
			r.addCheck("drive.quark.cookie", status, msg)
		}
	case "yun139":
		status, msg = validateOption(cfg.Drive.Yun139, "yun139")
		r.addCheck("drive.yun139", status, msg)
		if cfg.Drive.Yun139 != nil {
			status, msg = validateRequired("drive.yun139.authorization", cfg.Drive.Yun139.Authorization, true)
			r.addCheck("drive.yun139.authorization", status, msg)
		}
	case "localfs":
		status, msg = validateOption(cfg.Drive.LocalFS, "localfs")
		r.addCheck("drive.localfs", status, msg)
		if cfg.Drive.LocalFS != nil {
			status, msg = validateRequired("drive.localfs.root_path", cfg.Drive.LocalFS.RootPath, true)
			r.addCheck("drive.localfs.root_path", status, msg)
		}
	}

	// Max size parseable.
	if _, err := ParseSize(cfg.Cache.MaxSize); err != nil {
		r.addCheck("cache.max_size", "error", fmt.Sprintf("unable to parse: %s", err))
	} else {
		r.addCheck("cache.max_size", "ok", cfg.Cache.MaxSize)
	}

	// Dir cache TTL.
	if cfg.Sync.DirCacheTTL != "" {
		if _, err := ParseDuration(cfg.Sync.DirCacheTTL); err != nil {
			r.addCheck("sync.dir_cache_ttl", "error", fmt.Sprintf("unable to parse: %s", err))
		} else {
			r.addCheck("sync.dir_cache_ttl", "ok", cfg.Sync.DirCacheTTL)
		}
	}

	// Numeric ranges.
	if cfg.Sync.MaxRetries < 0 {
		r.addCheck("sync.max_retries", "error", "must not be negative")
	} else {
		r.addCheck("sync.max_retries", "ok", fmt.Sprintf("%d", cfg.Sync.MaxRetries))
	}
	if cfg.Sync.ConcurrentUploads <= 0 {
		r.addCheck("sync.concurrent_uploads", "error", "must be greater than 0")
	} else {
		r.addCheck("sync.concurrent_uploads", "ok", fmt.Sprintf("%d", cfg.Sync.ConcurrentUploads))
	}

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
	cfg, err := LoadConfig(path)
	if err != nil {
		r := &ValidationResult{
			Valid:    false,
			FilePath: path,
			Checks: []ValidationCheck{
				{Field: "config.file", Status: "error", Message: err.Error()},
			},
		}
		return r
	}
	r := ValidateConfig(cfg)
	r.FilePath = path
	return r
}

func (r *ValidationResult) addCheck(field, status, message string) {
	r.Checks = append(r.Checks, ValidationCheck{Field: field, Status: status, Message: message})
}

func validateDriveType(t string) (string, string) {
	switch t {
	case "quark", "yun139", "localfs":
		return "ok", t
	case "":
		return "error", "drive type not set (must be quark, yun139, or localfs)"
	default:
		return "error", fmt.Sprintf("unknown drive type: %s", t)
	}
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
