package platform

import (
	"context"
	"fmt"

	"github.com/yinzhenyu/skills/qrypt/cipher"
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

	if setter, ok := drv.(interface{ SetCipher(drivers.Cipher) }); ok {
		setter.SetCipher(ciph)
	}

	return qrypt.NewFileAPI(qrypt.Options{
		Cipher:        ciph,
		Dirs:          DesktopDirResolver{},
		Creds:         &TomlCredentialStore{cfg},
		DriverFactory: qrypt.SingleDriverFactory(drv),
	})
}

// NewFileAPIFromConfigForMount creates a FileAPI for a specific named mount.
// If mountName is empty, falls back to FindDefaultMount.
func NewFileAPIFromConfigForMount(cfg *config.Config, mountName, password, salt string) (*qrypt.FileAPI, error) {
	var mountCfg *config.MountInstance
	if mountName == "" {
		mountCfg = config.FindDefaultMount(cfg)
	} else {
		for _, m := range cfg.Mounts {
			if m.Name == mountName {
				mountCfg = &m
				break
			}
		}
	}
	if mountCfg == nil {
		if mountName != "" {
			return nil, fmt.Errorf("未找到挂载实例: %s", mountName)
		}
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

	if setter, ok := drv.(interface{ SetCipher(drivers.Cipher) }); ok {
		setter.SetCipher(ciph)
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
func NewFileAPIFromConfigWithAdapter(cfg *config.Config, drv drivers.Driver, ciph *cipher.RcloneCipher) (*qrypt.FileAPI, error) {
	return qrypt.NewFileAPI(qrypt.Options{
		Cipher:        ciph,
		Dirs:          DesktopDirResolver{},
		Creds:         &TomlCredentialStore{cfg},
		DriverFactory: qrypt.SingleDriverFactory(drv),
	})
}
