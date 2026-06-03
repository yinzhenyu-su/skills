// Package factory provides factory functions for creating backend.Driver instances from config.
package factory

import (
	"fmt"

	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/drivers/localfs"
	"github.com/yinzhenyu/skills/qrypt/drivers/quark"
	"github.com/yinzhenyu/skills/qrypt/drivers/yun139"
)

// NewDriverFromConfig creates a Driver from the given DriveConfig.
func NewDriverFromConfig(cfg config.DriveConfig) (backend.Driver, error) {
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
	case "localfs":
		if cfg.LocalFS == nil {
			return nil, fmt.Errorf("missing localfs config")
		}
		return localfs.NewDriver(cfg.LocalFS.RootPath), nil
	default:
		return nil, fmt.Errorf("unknown driver type: %q (supported: quark, yun139, localfs)", cfg.Type)
	}
}

// NewDriverFromType creates a Driver from a type string and MountParams.
func NewDriverFromType(driverType string, params config.MountParams) (backend.Driver, error) {
	switch driverType {
	case "quark":
		if params.Cookie == "" {
			return nil, fmt.Errorf("missing cookie for quark driver")
		}
		return quark.NewDriver(params.Cookie, params.RootPath), nil
	case "yun139":
		if params.Authorization == "" {
			return nil, fmt.Errorf("missing authorization for yun139 driver")
		}
		return yun139.NewDriver(params.Authorization, params.RootID), nil
	case "localfs":
		root := params.LocalRoot
		if root == "" {
			root = params.RootPath
		}
		if root == "" {
			return nil, fmt.Errorf("missing local_root for localfs driver")
		}
		return localfs.NewDriver(root), nil
	default:
		return nil, fmt.Errorf("unknown driver type: %q (supported: quark, yun139, localfs)", driverType)
	}
}
