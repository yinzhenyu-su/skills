// Package factory provides factory functions for creating drivers.Driver instances from config.
package factory

import (
	"github.com/yinzhenyu/skills/qrypt/drivers"
	"github.com/yinzhenyu/skills/qrypt/internal/config"

	// Side-effect imports: register all built-in drivers.
	_ "github.com/yinzhenyu/skills/qrypt/drivers/localfs"
	_ "github.com/yinzhenyu/skills/qrypt/drivers/quark"
	_ "github.com/yinzhenyu/skills/qrypt/drivers/yun139"
)

// NewDriverFromConfig creates a Driver from the given DriveConfig.
func NewDriverFromConfig(cfg config.DriveConfig) (drivers.Driver, error) {
	return drivers.New(cfg.Type, configToParams(cfg))
}

// NewDriverFromType creates a Driver from a type string and MountParams.
func NewDriverFromType(driverType string, params config.MountParams) (drivers.Driver, error) {
	return drivers.New(driverType, mountParamsToParams(params))
}

func configToParams(cfg config.DriveConfig) drivers.Params {
	p := drivers.Params{}
	switch {
	case cfg.Quark != nil:
		p["cookie"] = cfg.Quark.Cookie
		p["root_path"] = cfg.Quark.RootPath
	case cfg.Yun139 != nil:
		p["authorization"] = cfg.Yun139.Authorization
		p["root_id"] = cfg.Yun139.RootID
	case cfg.LocalFS != nil:
		p["local_root"] = cfg.LocalFS.RootPath
	}
	return p
}

func mountParamsToParams(mp config.MountParams) drivers.Params {
	return drivers.Params{
		"cookie":        mp.Cookie,
		"authorization": mp.Authorization,
		"root_path":     mp.RootPath,
		"root_id":       mp.RootID,
		"local_root":    mp.LocalRoot,
	}
}
