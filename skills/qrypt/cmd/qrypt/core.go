package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yinzhenyu/skills/qrypt/core/qrypt"
	"github.com/yinzhenyu/skills/qrypt/internal/config"
	"github.com/yinzhenyu/skills/qrypt/internal/platform"
)

func getCfg(cfgPath string) (*config.Config, error) {
	_, cfg, vr, err := config.LoadConfigAuto(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("加载配置失败: %w", err)
	}
	if !vr.Valid {
		return nil, fmt.Errorf("配置验证失败")
	}
	return cfg, nil
}

func newFileAPI(cfg *config.Config, password, salt string) (*qrypt.FileAPI, error) {
	return platform.NewFileAPIFromConfig(cfg, password, salt)
}

func newFileAPIFromCfgPath(cfgPath, password, salt string) (*qrypt.FileAPI, error) {
	cfg, err := getCfg(cfgPath)
	if err != nil {
		return nil, err
	}
	return newFileAPI(cfg, password, salt)
}

func apiFromCmd(cmd *cobra.Command) (*qrypt.FileAPI, error) {
	cfgPath, _ := cmd.Flags().GetString("config")
	password, _ := cmd.Flags().GetString("password")
	salt, _ := cmd.Flags().GetString("salt")
	return newFileAPIFromCfgPath(cfgPath, password, salt)
}

// newFileAPIForMount creates a FileAPI for a specific mount instance.
func newFileAPIForMount(cfg *config.Config, mountName, password, salt string) (*qrypt.FileAPI, error) {
	return platform.NewFileAPIFromConfigForMount(cfg, mountName, password, salt)
}

// apiFromCmdForMount creates a FileAPI for the named mount from cobra command flags.
func apiFromCmdForMount(cmd *cobra.Command, mountName string) (*qrypt.FileAPI, error) {
	cfgPath, _ := cmd.Flags().GetString("config")
	password, _ := cmd.Flags().GetString("password")
	salt, _ := cmd.Flags().GetString("salt")
	cfg, err := getCfg(cfgPath)
	if err != nil {
		return nil, err
	}
	return newFileAPIForMount(cfg, mountName, password, salt)
}
