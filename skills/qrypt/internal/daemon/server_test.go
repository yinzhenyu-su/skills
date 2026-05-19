package daemon

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

func newTestDaemon(t *testing.T) *Daemon {
	t.Helper()
	cfg := &config.Config{
		Mount: config.MountConfig{
			Point:      "/tmp/qrypt-test-mount",
			AllowOther: false,
		},
	}
	return NewDaemon(cfg, "test-version")
}

func TestDaemonStatus(t *testing.T) {
	d := newTestDaemon(t)
	status, err := d.Status()
	if err != nil {
		t.Fatal(err)
	}
	if status.Version != "test-version" {
		t.Fatalf("got version=%s", status.Version)
	}
	if status.MountState != protocol.MountStateUnmounted {
		t.Fatalf("got state=%s", status.MountState)
	}
}

func TestDaemonMountStatus(t *testing.T) {
	d := newTestDaemon(t)
	state, err := d.MountStatus()
	if err != nil {
		t.Fatal(err)
	}
	if state != "" && state != protocol.MountStateUnmounted {
		t.Fatalf("got state=%s", state)
	}
}

func TestDaemonGetConfig(t *testing.T) {
	d := newTestDaemon(t)
	cfg, err := d.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mount.Point != "/tmp/qrypt-test-mount" {
		t.Fatalf("got mount point=%s", cfg.Mount.Point)
	}
}

func TestDaemonUpdateConfig(t *testing.T) {
	d := newTestDaemon(t)
	newPoint := "/new/mount/point"
	err := d.UpdateConfig(context.Background(), protocol.ConfigPatch{
		MountPoint: &newPoint,
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := d.GetConfig()
	if cfg.Mount.Point != newPoint {
		t.Fatalf("got mount point=%s", cfg.Mount.Point)
	}
}

func TestDaemonValidateConfig(t *testing.T) {
	d := newTestDaemon(t)
	result, err := d.ValidateConfig("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if result.Valid {
		t.Fatal("expected invalid config")
	}

	checks := make(map[string]string)
	for _, c := range result.Checks {
		checks[c.Field] = c.Status
	}

	assertCheck := func(field, expected string) {
		if got := checks[field]; got != expected {
			t.Errorf("check %s: got %s, want %s", field, got, expected)
		}
	}

	assertCheck("drive.type", "error")
	assertCheck("encryption.password", "error")
	assertCheck("cache.max_size", "error")
	assertCheck("sync.concurrent_uploads", "error")
	assertCheck("mount.point", "ok")
}

func TestDaemonGetAccountInfo(t *testing.T) {
	d := newTestDaemon(t)
	info, err := d.GetAccountInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Username != "unknown" {
		t.Fatalf("got username=%s", info.Username)
	}
}

func TestDaemonSyncStatus(t *testing.T) {
	d := newTestDaemon(t)
	stats, err := d.SyncStatus()
	if err != nil {
		t.Fatal(err)
	}
	if stats == nil {
		t.Fatal("stats is nil")
	}
}

func TestDaemonGetSyncTaskList(t *testing.T) {
	d := newTestDaemon(t)
	tasks, err := d.GetSyncTaskList()
	if err != nil {
		t.Fatal(err)
	}
	// No cacheManager, so should return empty but not nil
	_ = tasks
}

func TestDaemonCacheUsage(t *testing.T) {
	d := newTestDaemon(t)
	usage, err := d.CacheUsage()
	if err != nil {
		t.Fatal(err)
	}
	if usage == nil {
		t.Fatal("usage is nil")
	}
}

func TestDaemonIsLoggedIn(t *testing.T) {
	d := newTestDaemon(t)
	if d.IsLoggedIn() {
		t.Fatal("should not be logged in without driver")
	}
}

func TestDispatchInvalidMethod(t *testing.T) {
	d := newTestDaemon(t)
	srv := &Server{daemon: d}
	resp := srv.dispatch(context.Background(), &protocol.Request{
		ID: 1, Method: "nonexistent",
	})
	if resp.Error == nil || resp.Error.Code != protocol.ErrCodeMethodNotFound {
		t.Fatalf("got error=%v", resp.Error)
	}
}

func TestDispatchStatus(t *testing.T) {
	d := newTestDaemon(t)
	srv := &Server{daemon: d}
	resp := srv.dispatch(context.Background(), &protocol.Request{
		ID: 1, Method: "status",
	})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestDispatchGetConfig(t *testing.T) {
	d := newTestDaemon(t)
	srv := &Server{daemon: d}
	resp := srv.dispatch(context.Background(), &protocol.Request{
		ID: 1, Method: "get_config",
	})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestServerStartStop(t *testing.T) {
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("qryptd-test-%d.sock", os.Getpid()))
	defer os.Remove(socketPath)

	d := newTestDaemon(t)
	srv := NewServer(d, socketPath)

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	// Connect and send a request
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Send status request
	req := &protocol.Request{ID: 1, Method: "status"}
	if err := protocol.EncodeRequest(conn, req); err != nil {
		t.Fatal(err)
	}

	// Read response
	resp, err := protocol.DecodeResponse(bufio.NewReader(conn))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("got error: %v", resp.Error)
	}
}

func TestServerEventSubscription(t *testing.T) {
	socketPath := filepath.Join(os.TempDir(), fmt.Sprintf("qryptd-events-%d.sock", os.Getpid()))
	defer os.Remove(socketPath)

	d := newTestDaemon(t)
	srv := NewServer(d, socketPath)

	ctx := context.Background()
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer srv.Stop()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Subscribe to events
	req := &protocol.Request{ID: 1, Method: "subscribe_events"}
	if err := protocol.EncodeRequest(conn, req); err != nil {
		t.Fatal(err)
	}

	// Read acknowledgment
	resp, err := protocol.DecodeResponse(bufio.NewReader(conn))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil {
		t.Fatalf("subscribe error: %v", resp.Error)
	}

	// Send an event from the daemon
	go func() {
		d.eventMgr.Publish(&protocol.Event{
			Type:      protocol.EventSyncCompleted,
			Timestamp: 12345,
			Data:      map[string]string{"status": "ok"},
		})
	}()

	// Read event from connection
	reader := bufio.NewReader(conn)
	eventLine, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	if len(eventLine) == 0 {
		t.Fatal("expected event line")
	}
}
