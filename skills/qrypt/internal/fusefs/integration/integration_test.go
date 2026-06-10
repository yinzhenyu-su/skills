//go:build integration

package integration

import (
	"context"
	"testing"
)

func TestConfiguredMounts(t *testing.T) {
	ctx := context.Background()
	cfg := ConfigOrDefault(t)
	mounts := cfg.DiscoverTestMounts()

	if len(mounts) == 0 {
		t.Skip("no mounts with test_enabled = true found in config")
	}

	for _, m := range mounts {
		m := m // capture
		t.Run(m.Name, func(t *testing.T) {
			drv := createDriver(t, m)
			if drv == nil {
				return
			}
			t.Cleanup(func() { drv.Drop(ctx) })
			runSuite(t, drv)
		})
	}
}
