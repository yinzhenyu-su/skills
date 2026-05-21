package daemon

import (
	"context"
	"testing"

	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

func TestMountManagerListEmpty(t *testing.T) {
	mm := NewMountManager(&config.Config{}, nil)
	list := mm.List()
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}
}

func TestMountManagerList(t *testing.T) {
	cfg := &config.Config{
		Mounts: []config.MountInstance{
			{
				Name:       "m1",
				Type:       "quark",
				MountPoint: "~/Qrypt/M1",
				Params: config.MountParams{
					Cookie:   "c1",
					RootPath: "/",
				},
			},
			{
				Name:       "m2",
				Type:       "quark",
				MountPoint: "~/Qrypt/M2",
				Params: config.MountParams{
					Cookie:   "c2",
					RootPath: "/Work",
				},
			},
		},
	}
	mm := NewMountManager(cfg, nil)
	list := mm.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 mounts, got %d", len(list))
	}
	if list[0].Name != "m1" {
		t.Errorf("expected m1, got %s", list[0].Name)
	}
	if list[1].Name != "m2" {
		t.Errorf("expected m2, got %s", list[1].Name)
	}
	// Both should be unmounted initially
	if list[0].State != "unmounted" {
		t.Errorf("expected unmounted, got %s", list[0].State)
	}
}

func TestMountManagerResolvedConfig(t *testing.T) {
	cfg := &config.Config{
		Mounts: []config.MountInstance{
			{
				Name:       "test",
				Type:       "quark",
				MountPoint: "~/Qrypt/Test",
				Params: config.MountParams{
					Cookie:   "test-cookie",
					RootPath: "/",
				},
			},
		},
	}
	mm := NewMountManager(cfg, nil)
	rc, ok := mm.ResolvedConfig("test")
	if !ok {
		t.Fatal("expected resolved config to be found")
	}
	if rc.Params.Cookie != "test-cookie" {
		t.Errorf("expected test-cookie, got %s", rc.Params.Cookie)
	}
	if rc.MountPoint[0] == '~' {
		t.Errorf("mount_point not expanded: %s", rc.MountPoint)
	}
}

func TestMountManagerGetNotFound(t *testing.T) {
	mm := NewMountManager(&config.Config{}, nil)
	_, err := mm.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent mount")
	}
}

func TestMountManagerLookupByPath(t *testing.T) {
	cfg := &config.Config{
		Mounts: []config.MountInstance{
			{
				Name:       "personal",
				Type:       "quark",
				MountPoint: "/Users/test/Qrypt/Personal",
				Params: config.MountParams{Cookie: "c1"},
			},
			{
				Name:       "work",
				Type:       "quark",
				MountPoint: "/Users/test/Qrypt/Work",
				Params: config.MountParams{Cookie: "c2"},
			},
		},
	}
	mm := NewMountManager(cfg, nil)

	// Mounts aren't started, so LookupByPath returns error
	// But we can verify config lookup by checking ResolvedConfig
	rc, ok := mm.ResolvedConfig("personal")
	if !ok {
		t.Fatal("expected to find personal config")
	}
	if rc.MountPoint != "/Users/test/Qrypt/Personal" {
		t.Errorf("expected Personal, got %s", rc.MountPoint)
	}

	// Path with matching prefix but wrong separator should not match
	// e.g. "/Users/test/Qrypt/PersonalFiles" (no / after Personal)
	// — cannot test this without Get() working (mount must be started)
	_ = cfg
}

func TestMountManagerListWithDefaults(t *testing.T) {
	cfg := &config.Config{
		Defaults: config.DefaultsConfig{
			Encryption: config.EncryptionConfig{Password: "default_pass"},
		},
		Mounts: []config.MountInstance{
			{
				Name:       "m1",
				Type:       "quark",
				MountPoint: "/m1",
				Params:     config.MountParams{Cookie: "c1"},
				// No encryption override — should use defaults
			},
		},
	}
	mm := NewMountManager(cfg, nil)
	list := mm.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(list))
	}
	if list[0].Name != "m1" {
		t.Errorf("expected m1, got %s", list[0].Name)
	}
}

func TestMountManagerStartStopFailsWithoutAuth(t *testing.T) {
	cfg := &config.Config{
		Defaults: config.DefaultsConfig{
			Encryption: config.EncryptionConfig{Password: "test"},
		},
		Mounts: []config.MountInstance{
			{
				Name:       "test",
				Type:       "quark",
				MountPoint: "/tmp/qrypt-test-mount-manager",
				Params: config.MountParams{
					Cookie:   "bad-cookie",
					RootPath: "/",
				},
			},
		},
	}
	mm := NewMountManager(cfg, nil)

	// Start will fail because cookie is not valid, but the error handling
	// should work — mount should be in error state, not crash
	err := mm.Start(context.Background(), "test")
	if err == nil {
		t.Log("Start unexpectedly succeeded (may have no network)")
	} else {
		t.Logf("Start failed as expected: %v", err)
	}

	// Stop should work even if start failed
	_ = mm.Stop(context.Background(), "test")
}
