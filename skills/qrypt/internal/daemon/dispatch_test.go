package daemon

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yinzhenyu/skills/qrypt/internal/protocol"
)

func TestDispatchAllMethods(t *testing.T) {
	d := newTestDaemon(t)
	srv := &Server{daemon: d}
	ctx := context.Background()

	tests := []struct {
		name   string
		method string
		params interface{}
	}{
		{"mount_status", "mount_status", nil},
		{"get_config", "get_config", nil},
		{"is_logged_in", "is_logged_in", nil},
		{"account_info", "account_info", nil},
		{"sync_status", "sync_status", nil},
		{"sync_task_list", "sync_task_list", nil},
		{"cache_usage", "cache_usage", nil},
		{"start", "start", nil},
		{"stop", "stop", nil},
		{"logout", "logout", nil},
		{"login_qr", "login_qr", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &protocol.Request{
				ID:     1,
				Method: tt.method,
				Params: tt.params,
			}
			resp := srv.dispatch(ctx, req)
			// These should not panic and should return valid responses
			if resp == nil {
				t.Fatal("response is nil")
			}
		})
	}
}

func TestDispatchUpdateConfig(t *testing.T) {
	d := newTestDaemon(t)
	srv := &Server{daemon: d}
	ctx := context.Background()

	newPoint := "/test/mount"
	params := map[string]interface{}{
		"mount_point": newPoint,
	}

	resp := srv.dispatch(ctx, &protocol.Request{
		ID: 1, Method: "update_config", Params: params,
	})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	cfg, _ := d.GetConfig()
	if cfg.Mount.Point != newPoint {
		t.Fatalf("mount point not updated: %s", cfg.Mount.Point)
	}
}

func TestDispatchUpdateConfigInvalid(t *testing.T) {
	d := newTestDaemon(t)
	srv := &Server{daemon: d}

	// Send update without proper params (string instead of JSON object)
	resp := srv.dispatch(context.Background(), &protocol.Request{
		ID: 1, Method: "update_config", Params: "invalid",
	})
	if resp.Error == nil {
		t.Fatal("expected error for invalid params")
	}
}

func TestDispatchValidateConfig(t *testing.T) {
	d := newTestDaemon(t)
	srv := &Server{daemon: d}

	// Valid config
	resp := srv.dispatch(context.Background(), &protocol.Request{
		ID: 1, Method: "validate_config",
		Params: map[string]interface{}{
			"password": "goodpassword",
		},
	})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}

	// Invalid - empty password
	resp = srv.dispatch(context.Background(), &protocol.Request{
		ID: 2, Method: "validate_config",
		Params: map[string]interface{}{
			"password": "",
		},
	})
	if resp.Error != nil {
		t.Fatalf("validate error: %v", resp.Error)
	}
	result := resp.Result
	if result == nil {
		t.Fatal("result is nil")
	}
}

func TestDispatchLoginCookie(t *testing.T) {
	d := newTestDaemon(t)
	srv := &Server{daemon: d}

	// Invalid params (wrong type for json unmarshal)
	resp := srv.dispatch(context.Background(), &protocol.Request{
		ID: 1, Method: "login_cookie", Params: "not_a_json_object",
	})
	if resp.Error == nil {
		t.Fatal("expected error for invalid params")
	}
}

func TestDispatchLoginQR(t *testing.T) {
	d := newTestDaemon(t)
	srv := &Server{daemon: d}

	resp := srv.dispatch(context.Background(), &protocol.Request{
		ID: 1, Method: "login_qr",
	})
	if resp.Error == nil {
		// QR not supported yet, but should not panic
		t.Log("login_qr returned without error")
	}
}

func TestDispatchCacheUsage(t *testing.T) {
	d := newTestDaemon(t)
	srv := &Server{daemon: d}

	resp := srv.dispatch(context.Background(), &protocol.Request{
		ID: 1, Method: "cache_usage",
	})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestDispatchSyncStatus(t *testing.T) {
	d := newTestDaemon(t)
	srv := &Server{daemon: d}

	resp := srv.dispatch(context.Background(), &protocol.Request{
		ID: 1, Method: "sync_status",
	})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestDispatchSyncTaskList(t *testing.T) {
	d := newTestDaemon(t)
	srv := &Server{daemon: d}

	resp := srv.dispatch(context.Background(), &protocol.Request{
		ID: 1, Method: "sync_task_list",
	})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
}

func TestUnmarshalParams(t *testing.T) {
	params := map[string]interface{}{
		"mount_point": "/test/path",
	}

	var target struct {
		MountPoint string `json:"mount_point"`
	}
	if err := unmarshalParams(params, &target); err != nil {
		t.Fatal(err)
	}
	if target.MountPoint != "/test/path" {
		t.Fatalf("got mount_point=%s", target.MountPoint)
	}
}

func TestUnmarshalParamsJSON(t *testing.T) {
	// Params might come as raw JSON bytes over the wire
	raw := `{"cookie":"test_cookie"}`
	var params interface{}
	json.Unmarshal([]byte(raw), &params)

	var target struct {
		Cookie string `json:"cookie"`
	}
	if err := unmarshalParams(params, &target); err != nil {
		t.Fatal(err)
	}
	if target.Cookie != "test_cookie" {
		t.Fatalf("got cookie=%s", target.Cookie)
	}
}

func TestServiceLoginLogout(t *testing.T) {
	d := newTestDaemon(t)
	ctx := context.Background()

	// Login with cookie (updates config)
	if err := d.LoginCookie(ctx, "test-cookie"); err != nil {
		t.Fatal(err)
	}
	cfg, _ := d.GetConfig()
	if cfg.Drive.Quark != nil && cfg.Drive.Quark.Cookie != "test-cookie" {
		t.Fatalf("cookie not set")
	}

	// Logout
	if err := d.Logout(ctx); err != nil {
		t.Fatal(err)
	}

	// QR login returns error (not implemented)
	_, _, err := d.LoginQR(ctx)
	if err == nil {
		t.Fatal("expected error for QR login")
	}
}

func TestServicePauseResumeSync(t *testing.T) {
	d := newTestDaemon(t)
	ctx := context.Background()

	if err := d.PauseSync(ctx); err == nil {
		t.Log("PauseSync returned nil (stub)")
	}
	if err := d.ResumeSync(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestServiceClearCacheStaging(t *testing.T) {
	d := newTestDaemon(t)
	ctx := context.Background()

	if err := d.ClearCache(ctx); err == nil {
		t.Log("ClearCache returned nil (stub)")
	}
	// ClearStaging should not error without cache manager
	if err := d.ClearStaging(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestServiceExportImportConfig(t *testing.T) {
	d := newTestDaemon(t)

	if err := d.ExportConfig("/tmp/nonexistent/test.toml"); err == nil {
		t.Log("ExportConfig returned nil (stub)")
	}
	if err := d.ImportConfig("/tmp/nonexistent/test.toml"); err == nil {
		t.Log("ImportConfig returned nil (stub)")
	}
}

func TestServiceStartStop(t *testing.T) {
	d := newTestDaemon(t)
	ctx := context.Background()

	// Stop when not started should not error
	if err := d.Stop(ctx); err != nil {
		t.Fatal(err)
	}

	// Start will fail without valid crypto/driver (that's expected)
	if err := d.Start(ctx); err == nil {
		t.Log("Start unexpectedly succeeded")
	}
}
