# Driver Configuration Specification

## Purpose

Define how storage backends are selected and configured via TOML configuration, supporting multiple drive types (Quark, Yun139) with independent parameter sections and backward compatibility.

## Requirements

### Requirement: 驱动选择
系统 SHALL 通过 `drive.type` 配置项选择使用的存储后端。

#### Scenario: 选择 Quark 驱动
- **WHEN** `drive.type` 设置为 `"quark"`
- **THEN** 系统使用 Quark 驱动进行所有存储操作

#### Scenario: 选择 139 驱动
- **WHEN** `drive.type` 设置为 `"yun139"`
- **THEN** 系统使用天翼云盘 139 驱动进行所有存储操作

### Requirement: 驱动参数配置
每种驱动 SHALL 有独立的 TOML 配置节，参数在该节下定义。

#### Scenario: Quark 参数
- **WHEN** `[drive]` 节中 `type = "quark"`
- **THEN** 配置 SHALL 包含 `[drive.quark]` 节
- **AND** 该节 SHALL 支持 `cookie` 和 `root_path` 字段

#### Scenario: 139 参数
- **WHEN** `[drive]` 节中 `type = "yun139"`
- **THEN** 配置 SHALL 包含 `[drive.yun139]` 节
- **AND** 该节 SHALL 支持 `authorization` 和 `root_id` 字段

### Requirement: 工厂函数
系统 SHALL 提供一个工厂函数，根据配置选择并实例化对应的驱动。

#### Scenario: 工厂函数返回正确驱动
- **WHEN** 传入 `type = "quark"` 的配置
- **THEN** 工厂函数返回 `*drive/quark.QuarkDriver`
- **AND** 驱动已使用配置中的参数初始化

#### Scenario: 未知驱动类型
- **WHEN** 传入一个未注册的 `drive.type`
- **THEN** 工厂函数返回错误

### Requirement: 向后兼容
系统 SHALL 支持旧版 `[quark]` 顶层配置格式，不做 breaking change。

#### Scenario: 旧格式自动迁移
- **WHEN** 配置文件使用旧格式（顶级 `[quark]` 节且无 `[drive]` 节）
- **THEN** 系统自动识别为 Quark 驱动
- **AND** 从旧 `[quark]` 节读取 `cookie` 和 `root_path`
