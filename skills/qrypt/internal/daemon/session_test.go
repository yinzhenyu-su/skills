package daemon

import (
	"context"
	"testing"

	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

func TestSessionManagerAcquireNew(t *testing.T) {
	sm := NewSessionManager()
	ctx := context.Background()
	key := SessionKey{Type: "localfs"}
	params := config.MountParams{LocalRoot: t.TempDir()}

	s, err := sm.Acquire(ctx, key, params)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if s.Drv == nil {
		t.Fatal("expected non-nil driver")
	}
}

func TestSessionManagerLocalFSNotPooled(t *testing.T) {
	sm := NewSessionManager()
	ctx := context.Background()
	key := SessionKey{Type: "localfs"}
	params := config.MountParams{LocalRoot: t.TempDir()}

	s1, _ := sm.Acquire(ctx, key, params)
	s2, _ := sm.Acquire(ctx, key, params)
	if s1.Drv == s2.Drv {
		t.Fatal("expected fresh driver for localfs (not pooled)")
	}
}

func TestSessionManagerReleaseNotPooledIsNoop(t *testing.T) {
	sm := NewSessionManager()
	ctx := context.Background()
	key := SessionKey{Type: "localfs"}
	params := config.MountParams{LocalRoot: t.TempDir()}

	s, _ := sm.Acquire(ctx, key, params)

	// Release for non-pooled sessions = no-op (no panic).
	sm.Release(ctx, key)
	sm.Release(ctx, key)

	_ = s
}

func TestSessionKeyForMount_Quark(t *testing.T) {
	rc := &config.ResolvedMountConfig{
		Type: "quark",
		Params: config.MountParams{
			Cookie:   "test_cookie",
			RootPath: "/",
		},
	}
	sk, pooled := SessionKeyForMount(rc)
	if !pooled {
		t.Fatal("expected quark session to be pooled")
	}
	if sk.Type != "quark" {
		t.Fatalf("expected type 'quark', got %q", sk.Type)
	}
	if sk.CredKey == "" {
		t.Fatal("expected non-empty cred key")
	}

	// Same cookie → same cred key (root_path doesn't matter)
	rc2 := &config.ResolvedMountConfig{
		Type: "quark",
		Params: config.MountParams{
			Cookie:   "test_cookie",
			RootPath: "/different",
		},
	}
	sk2, _ := SessionKeyForMount(rc2)
	if sk.CredKey != sk2.CredKey {
		t.Fatal("expected same cred key for same cookie regardless of root path")
	}
}

func TestSessionKeyForMount_LocalFS(t *testing.T) {
	rc := &config.ResolvedMountConfig{
		Type: "localfs",
		Params: config.MountParams{
			LocalRoot: "/tmp",
		},
	}
	_, pooled := SessionKeyForMount(rc)
	if pooled {
		t.Fatal("expected localfs NOT to be pooled")
	}
}

func TestSessionKeyForMount_Yun139(t *testing.T) {
	rc := &config.ResolvedMountConfig{
		Type: "yun139",
		Params: config.MountParams{
			Authorization: "token_abc",
			RootID:        "root",
		},
	}
	sk, pooled := SessionKeyForMount(rc)
	if !pooled {
		t.Fatal("expected yun139 session to be pooled")
	}
	if sk.Type != "yun139" {
		t.Fatalf("expected type 'yun139', got %q", sk.Type)
	}

	// Same token → same cred key
	rc2 := &config.ResolvedMountConfig{
		Type: "yun139",
		Params: config.MountParams{
			Authorization: "token_abc",
			RootID:        "other_root",
		},
	}
	sk2, _ := SessionKeyForMount(rc2)
	if sk.CredKey != sk2.CredKey {
		t.Fatal("expected same cred key for same token regardless of root id")
	}

	// Different token → different cred key
	rc3 := &config.ResolvedMountConfig{
		Type: "yun139",
		Params: config.MountParams{
			Authorization: "token_xyz",
			RootID:        "root",
		},
	}
	sk3, _ := SessionKeyForMount(rc3)
	if sk.CredKey == sk3.CredKey {
		t.Fatal("expected different cred key for different tokens")
	}
}
