package platform

import (
	"context"
	"fmt"

	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	"github.com/yinzhenyu/skills/qrypt/drivers"
	factory "github.com/yinzhenyu/skills/qrypt/drivers/factory"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
)

// NewFileAPIFromConfig wires the desktop FileAPI from the loaded TOML config.
// Used by cmd/qrypt for CLI subcommands.
func NewFileAPIFromConfig(cfg *config.Config, password, salt string) (*qrypt.FileAPI, error) {
	mountCfg := config.FindDefaultMount(cfg)
	if mountCfg == nil {
		return nil, fmt.Errorf("配置中未找到启用的挂载实例")
	}

	rc := cfg.MergeInstanceConfig(*mountCfg)

	ciph, err := config.MakeCipher(rc.Encryption, cfg.Defaults.Encryption, password, salt)
	if err != nil {
		return nil, fmt.Errorf("创建加密引擎失败: %w", err)
	}

	drv, err := factory.NewDriverFromType(rc.Type, rc.Params)
	if err != nil {
		return nil, fmt.Errorf("创建驱动失败: %w", err)
	}

	if err := drv.Init(context.Background()); err != nil {
		return nil, fmt.Errorf("驱动初始化失败: %w", err)
	}

	return qrypt.NewFileAPI(qrypt.Options{
		Cipher:        ciph,
		Dirs:          DesktopDirResolver{},
		Creds:         &TomlCredentialStore{cfg},
		DriverFactory: qrypt.SingleDriverFactory(drv),
	})
}

// NewFileAPIFromConfigWithAdapter is used when the caller already has a constructed
// backend driver (e.g., daemon hot-path reuse).
func NewFileAPIFromConfigWithAdapter(cfg *config.Config, drv backend.Driver, ciph *qrypt.RcloneCipher) (*qrypt.FileAPI, error) {
	return qrypt.NewFileAPI(qrypt.Options{
		Cipher:        ciph,
		Dirs:          DesktopDirResolver{},
		Creds:         &TomlCredentialStore{cfg},
		DriverFactory: qrypt.SingleDriverFactory(drv),
	})
}
