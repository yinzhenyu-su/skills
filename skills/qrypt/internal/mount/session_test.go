package mount

import (
	"context"
	"testing"

	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/coreadapter"
)

func TestSessionKeyForMount_Quark(t *testing.T) {
	rc := &config.ResolvedMountConfig{
		Type: "quark",
		Params: config.MountParams{
			Cookie:   "test_cookie",
			RootPath: "/",
		},
	}
	sk := SessionKeyForMount(rc)
	if sk.Type != "quark" {
		t.Fatalf("expected type 'quark', got %q", sk.Type)
	}
	if sk.CredKey == "" {
		t.Fatal("expected non-empty cred key")
	}

	rc2 := &config.ResolvedMountConfig{
		Type: "quark",
		Params: config.MountParams{
			Cookie:   "test_cookie",
			RootPath: "/different",
		},
	}
	sk2 := SessionKeyForMount(rc2)
	if sk.CredKey != sk2.CredKey {
		t.Fatal("expected same cred key for same cookie regardless of root path")
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
	sk := SessionKeyForMount(rc)
	if sk.Type != "yun139" {
		t.Fatalf("expected type 'yun139', got %q", sk.Type)
	}

	rc3 := &config.ResolvedMountConfig{
		Type: "yun139",
		Params: config.MountParams{
			Authorization: "token_xyz",
			RootID:        "root",
		},
	}
	sk3 := SessionKeyForMount(rc3)
	if sk.CredKey == sk3.CredKey {
		t.Fatal("expected different cred key for different tokens")
	}
}

func TestSessionKeyForMount_LocalFS_DistinctPaths(t *testing.T) {
	rc1 := &config.ResolvedMountConfig{
		Type:   "localfs",
		Params: config.MountParams{LocalRoot: "/tmp/a"},
	}
	rc2 := &config.ResolvedMountConfig{
		Type:   "localfs",
		Params: config.MountParams{LocalRoot: "/tmp/b"},
	}
	sk1 := SessionKeyForMount(rc1)
	sk2 := SessionKeyForMount(rc2)
	if sk1.CredKey == sk2.CredKey {
		t.Fatal("expected different cred keys for different localfs paths")
	}
}

func TestDriverFactory_LocalFS(t *testing.T) {
	df := NewDriverFactory()
	df.Register("test_local", config.MountParams{LocalRoot: t.TempDir()})

	cfg := qrypt.SessionConfig{Type: "localfs:test_local"}
	drv, err := df.CreateDriver(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CreateDriver: %v", err)
	}
	if drv == nil {
		t.Fatal("nil driver returned")
	}
	if _, ok := drv.(*coreadapter.DriverAdapter); !ok {
		t.Errorf("expected *coreadapter.DriverAdapter, got %T", drv)
	}
}

func TestDriverFactory_UnregisteredMount(t *testing.T) {
	df := NewDriverFactory()
	cfg := qrypt.SessionConfig{Type: "localfs:missing"}
	if _, err := df.CreateDriver(context.Background(), cfg); err == nil {
		t.Fatal("expected error for unregistered mount")
	}
}

func TestDriverFactory_UnknownType(t *testing.T) {
	df := NewDriverFactory()
	df.Register("x", config.MountParams{LocalRoot: t.TempDir()})
	cfg := qrypt.SessionConfig{Type: "unknown:x"}
	if _, err := df.CreateDriver(context.Background(), cfg); err == nil {
		t.Fatal("expected error for unknown backend type")
	}
}

func TestSplitTypeMount(t *testing.T) {
	cases := []struct {
		in       string
		wantType string
		wantName string
	}{
		{"localfs:test_local", "localfs", "test_local"},
		{"quark:photos", "quark", "photos"},
		{"yun139:media", "yun139", "media"},
		{"quark", "quark", "quark"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			ty, mn := splitTypeMount(c.in)
			if ty != c.wantType || mn != c.wantName {
				t.Errorf("got (%q,%q), want (%q,%q)", ty, mn, c.wantType, c.wantName)
			}
		})
	}
}

func TestSessionManagerIntegration_LocalFS(t *testing.T) {
	// Verify qrypt.SessionManager pools localfs sessions by LocalRoot (different
	// roots = different sessions; same root = pooled).
	df := NewDriverFactory()
	sm := qrypt.NewSessionManager(df)

	root1 := t.TempDir()
	df.Register("a", config.MountParams{LocalRoot: root1})
	df.Register("b", config.MountParams{LocalRoot: root1})

	rcA := &config.ResolvedMountConfig{Type: "localfs", Params: config.MountParams{LocalRoot: root1}}
	rcB := &config.ResolvedMountConfig{Type: "localfs", Params: config.MountParams{LocalRoot: root1}}

	keyA := SessionKeyForMount(rcA)
	keyB := SessionKeyForMount(rcB)
	if keyA != keyB {
		t.Fatal("expected same key for same root")
	}

	ctx := context.Background()
	sA, err := sm.Acquire(ctx, keyA, SessionConfigForMount("a", rcA))
	if err != nil {
		t.Fatal(err)
	}
	sB, err := sm.Acquire(ctx, keyB, SessionConfigForMount("b", rcB))
	if err != nil {
		t.Fatal(err)
	}
	if sA != sB {
		t.Error("expected same pooled session for matching key")
	}
	sm.Release(ctx, keyA)
	sm.Release(ctx, keyB)
}
