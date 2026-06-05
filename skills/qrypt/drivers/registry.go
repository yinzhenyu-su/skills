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

var (
	mu       sync.RWMutex
	registry = make(map[string]Constructor)
)

// Register makes a driver constructor available by name.
// Panics on duplicate registration (same behavior as database/sql).
// Typically called from init() in each driver package.
func Register(name string, ctor Constructor) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := registry[name]; dup {
		panic("drivers: Register called twice for driver " + name)
	}
	registry[name] = ctor
}

// New creates a Driver by looking up the registered constructor for the given name.
func New(name string, params Params) (Driver, error) {
	mu.RLock()
	ctor, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown driver type: %q (registered: %s)", name, strings.Join(SupportedTypes(), ", "))
	}
	return ctor(params)
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
