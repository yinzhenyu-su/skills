//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"testing"

	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/drivers/localfs"
	"github.com/yinzhenyu/skills/qrypt/drivers/quark"
	"github.com/yinzhenyu/skills/qrypt/drivers/yun139"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

// DriverFactory creates a drivers.Driver from a mount config.Params.
type DriverFactory func(params map[string]string) (drivers.Driver, error)

var driverFactories = map[string]DriverFactory{
	"quark": func(params map[string]string) (drivers.Driver, error) {
		cookie := params["cookie"]
		if cookie == "" {
			return nil, fmt.Errorf("quark driver requires cookie param")
		}
		rootPath := params["root_path"]
		if rootPath == "" {
			rootPath = "/"
		}
		dirCacheTTL, _ := time.ParseDuration(params["dir_cache_ttl"])
		return quark.NewDriver(cookie, rootPath, dirCacheTTL), nil
	},
	"yun139": func(params map[string]string) (drivers.Driver, error) {
		auth := params["authorization"]
		if auth == "" {
			return nil, fmt.Errorf("yun139 driver requires authorization param")
		}
		return yun139.NewDriver(auth, params["root_id"]), nil
	},
	"localfs": func(params map[string]string) (drivers.Driver, error) {
		root := params["local_root"]
		if root == "" {
			return nil, fmt.Errorf("localfs driver requires local_root param")
		}
		return localfs.NewDriver(root), nil
	},
}

// createDriver instantiates a driver from a MountInstance.
// It calls Init to validate credentials. On failure (bad cookie, network),
// it returns nil — caller should t.Skip.
func createDriver(t testing.TB, m config.MountInstance) drivers.Driver {
	factory, ok := driverFactories[m.Type]
	if !ok {
		t.Skipf("unsupported driver type: %s (no factory registered)", m.Type)
		return nil
	}

	drv, err := factory(m.Params)
	if err != nil {
		t.Skipf("create %s driver: %v", m.Type, err)
		return nil
	}

	if err := drv.Init(context.Background()); err != nil {
		if isAuthError(err) {
			t.Skipf("%s driver auth failed (cookie expired?): %v", m.Type, err)
			return nil
		}
		if isNetworkError(err) {
			t.Skipf("%s driver network error (offline?): %v", m.Type, err)
			return nil
		}
		t.Fatalf("%s driver init: %v", m.Type, err)
	}

	return drv
}

// isAuthError checks if an error is likely an authentication failure.
func isAuthError(err error) bool {
	s := err.Error()
	return strings.Contains(s, "cookie") ||
		strings.Contains(s, "auth") ||
		strings.Contains(s, "401") ||
		strings.Contains(s, "403")
}

// isNetworkError checks if an error is likely a network issue.
func isNetworkError(err error) bool {
	s := err.Error()
	return strings.Contains(s, "no such host") ||
		strings.Contains(s, "connection refused") ||
		strings.Contains(s, "timeout") ||
		strings.Contains(s, "network")
}

// TestRoot creates a unique temporary directory on the driver for testing.
// Returns the directory path (parent FID) and a cleanup function.
func TestRoot(ctx context.Context, t testing.TB, drv drivers.Driver) (string, func()) {
	// The parent FID "0" is the root of the mount.
	// Create a subdirectory with a unique name so parallel tests don't collide.
	dirName := fmt.Sprintf(".integration-test-%d", os.Getpid())
	if w, ok := drv.(drivers.Writer); ok {
		entry, err := w.Mkdir(ctx, "0", dirName)
		if err != nil {
			t.Fatalf("create test root: %v", err)
		}
		return entry.ID, func() {
			removeAll(ctx, drv, entry.ID)
		}
	}
	return "0", func() {}
}

// removeAll recursively removes a directory and its contents.
func removeAll(ctx context.Context, drv drivers.Driver, fid string) {
	w, ok := drv.(drivers.Writer)
	if !ok {
		return
	}
	entries, err := drv.List(ctx, fid)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir {
			removeAll(ctx, drv, e.ID)
		}
		_ = w.Remove(ctx, e)
	}
	_ = w.Remove(ctx, drivers.Entry{ID: fid})
}

// RootPath extracts the "root_path" param or returns "/".
func RootPath(m config.MountInstance) string {
	if v := m.Params["root_path"]; v != "" {
		return v
	}
	return "/"
}

// ConfigOrDefault loads the qrypt config from the default location,
// or returns a config with test_enabled mounts from env vars if no config found.
func ConfigOrDefault(t testing.TB) *config.Config {
	cfgPath := config.FindConfigFile()
	if cfgPath == "" {
		// Also check the default work dir (~/.qrypt/qrypt.toml).
		if _, err := os.Stat(filepath.Join(config.WorkDir(), "qrypt.toml")); err == nil {
			cfgPath = filepath.Join(config.WorkDir(), "qrypt.toml")
		}
	}
	if cfgPath == "" {
		t.Log("no config file found, using env-based defaults")
		return envBasedConfig()
	}
	cfg, _, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}

// envBasedConfig creates a minimal config from environment variables.
// This allows running integration tests without a config file.
func envBasedConfig() *config.Config {
	cfg := config.DefaultConfig()
	if cookie := os.Getenv("QRYPT_COOKIE"); cookie != "" {
		enabled := true
		cfg.Mounts = append(cfg.Mounts, config.MountInstance{
			Name:        "quark",
			Type:        "quark",
			TestEnabled: &enabled,
			Params: map[string]string{
				"cookie":    cookie,
				"root_path": envOrDefault("QRYPT_ROOT_PATH", "/"),
			},
		})
	}
	if auth := os.Getenv("YUN139_AUTH"); auth != "" {
		enabled := true
		cfg.Mounts = append(cfg.Mounts, config.MountInstance{
			Name:        "yun139",
			Type:        "yun139",
			TestEnabled: &enabled,
			Params: map[string]string{
				"authorization": auth,
			},
		})
	}
	return cfg
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ResolvePath resolves a logical path like "/dir/file.txt" to a FID
// on the driver, starting from parentFID. Returns the FID of the target.
func ResolvePath(ctx context.Context, drv drivers.Driver, parentFID string, path string) (string, error) {
	clean := filepath.Clean(path)
	clean = strings.TrimPrefix(clean, "/")
	if clean == "" || clean == "." {
		return parentFID, nil
	}
	segments := strings.Split(clean, "/")
	current := parentFID
	for _, seg := range segments {
		entries, err := drv.List(ctx, current)
		if err != nil {
			return "", fmt.Errorf("list %s: %w", seg, err)
		}
		found := false
		for _, e := range entries {
			if e.Name == seg {
				current = e.ID
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("path segment %s not found under %s", seg, current)
		}
	}
	return current, nil
}
