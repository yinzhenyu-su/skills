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

// NewDriverFromType creates a Driver from a type string and MountParams.
func NewDriverFromType(driverType string, params config.MountParams) (drivers.Driver, error) {
	return drivers.New(driverType, drivers.Params(params))
}
