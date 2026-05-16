## ADDED Requirements

### Requirement: 配置迁移
Quark 驱动的配置从顶层 `[quark]` 节迁移到 `[drive.quark]` 节。

#### Scenario: 新配置格式
- **WHEN** 用户使用新格式配置 139 驱动
- **THEN** `[drive]` 节中的 `type = "quark"` 和 `[drive.quark]` 节生效
- **AND** 顶层 `[quark]` 节不再使用

#### Scenario: 旧格式仍然可用
- **WHEN** 用户未更新配置文件
- **THEN** 顶层 `[quark]` 节仍然被识别
- **AND** 系统打印提示建议迁移到新格式
