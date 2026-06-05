package drivers

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Params is a flat key-value bag passed to driver constructors.
// Each driver defines its own required/optional keys (e.g. "cookie", "root_path").
type Params map[string]string

// Constructor creates a Driver from the given Params.
// Implementations should validate required params and return descriptive errors.
type Constructor func(params Params) (Driver, error)

// ParamSpec describes a single driver parameter for validation and UI generation.
type ParamSpec struct {
	Key      string `json:"key"`                // TOML/JSON key, e.g. "cookie"
	Required bool   `json:"required,omitempty"` // must be provided
	Help     string `json:"help,omitempty"`     // human-readable description
	Default  string `json:"default,omitempty"`  // default value
	Type     string `json:"type,omitempty"`     // "string" | "select" | "bool" (default: "string")
	Options  string `json:"options,omitempty"`  // comma-separated for select type
}

// DriverMeta holds the constructor and parameter metadata for a registered driver.
type DriverMeta struct {
	Ctor          Constructor // creates a Driver from Params
	Params        []ParamSpec // declared parameters
	RootKey       string      // which param key holds the root path/ID (e.g. "root_path")
	CredentialKey string      // which param key holds the credential for session dedup (e.g. "cookie")
}

var (
	mu       sync.RWMutex
	registry = make(map[string]DriverMeta)
)

// Register makes a driver available by name with its metadata.
// Panics on duplicate registration (same behavior as database/sql).
// Typically called from init() in each driver package.
func Register(name string, meta DriverMeta) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := registry[name]; dup {
		panic("drivers: Register called twice for driver " + name)
	}
	registry[name] = meta
}

// New creates a Driver by looking up the registered constructor for the given name.
func New(name string, params Params) (Driver, error) {
	mu.RLock()
	meta, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown driver type: %q (registered: %s)", name, strings.Join(SupportedTypes(), ", "))
	}
	return meta.Ctor(params)
}

// GetMeta returns the DriverMeta for the given driver type name.
func GetMeta(name string) (DriverMeta, bool) {
	mu.RLock()
	defer mu.RUnlock()
	meta, ok := registry[name]
	return meta, ok
}

// SupportedTypes returns a sorted list of registered driver type names.
func SupportedTypes() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
