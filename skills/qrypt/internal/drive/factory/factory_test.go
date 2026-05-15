package factory

import (
	"testing"

	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

func TestNewDriverFromConfig_UnknownType(t *testing.T) {
	cfg := config.DriveConfig{Type: "unknown"}
	_, err := NewDriverFromConfig(cfg)
	if err == nil {
		t.Fatal("expected error for unknown driver type")
	}
}

func TestNewDriverFromConfig_MissingQuarkConfig(t *testing.T) {
	cfg := config.DriveConfig{Type: "quark"}
	_, err := NewDriverFromConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing quark config")
	}
}

func TestNewDriverFromConfig_MissingYun139Config(t *testing.T) {
	cfg := config.DriveConfig{Type: "yun139"}
	_, err := NewDriverFromConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing yun139 config")
	}
}

func TestNewDriverFromConfig_QuarkSuccess(t *testing.T) {
	cfg := config.DriveConfig{
		Type: "quark",
		Quark: &config.QuarkOptions{
			Cookie:   "test_cookie",
			RootPath: "/",
		},
	}
	drv, err := NewDriverFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if drv == nil {
		t.Fatal("expected non-nil driver")
	}
}

func TestNewDriverFromConfig_Yun139Success(t *testing.T) {
	cfg := config.DriveConfig{
		Type: "yun139",
		Yun139: &config.Yun139Options{
			Authorization: "test_auth",
			RootID:        "0",
		},
	}
	drv, err := NewDriverFromConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if drv == nil {
		t.Fatal("expected non-nil driver")
	}
}

func TestNewDriverFromConfig_EmptyType(t *testing.T) {
	cfg := config.DriveConfig{}
	_, err := NewDriverFromConfig(cfg)
	if err == nil {
		t.Fatal("expected error for empty driver type")
	}
}
