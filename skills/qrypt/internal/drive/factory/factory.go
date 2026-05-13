// Package factory provides factory functions for creating drive.Driver instances from config.
package factory

import (
	"fmt"

	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/drive"
	"github.com/yinzhenyu/skills/qrypt/internal/drive/quark"
	"github.com/yinzhenyu/skills/qrypt/internal/drive/yun139"
)

// NewDriverFromConfig creates a Driver from the given DriveConfig.
func NewDriverFromConfig(cfg config.DriveConfig) (drive.Driver, error) {
	switch cfg.Type {
	case "quark":
		if cfg.Quark == nil {
			return nil, fmt.Errorf("missing quark config")
		}
		return quark.NewDriver(cfg.Quark.Cookie, cfg.Quark.RootPath), nil
	case "yun139":
		if cfg.Yun139 == nil {
			return nil, fmt.Errorf("missing yun139 config")
		}
		return yun139.NewDriver(cfg.Yun139.Authorization, cfg.Yun139.RootID), nil
	default:
		return nil, fmt.Errorf("unknown driver type: %q (supported: quark, yun139)", cfg.Type)
	}
}
