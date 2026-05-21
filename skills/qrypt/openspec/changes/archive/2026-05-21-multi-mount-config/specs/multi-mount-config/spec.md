## ADDED Requirements

### Requirement: 多实例 TOML 配置模型

系统 SHALL 支持通过 TOML 配置文件声明多个网盘挂载实例。

#### Scenario: 声明两个 Quark 实例
- **WHEN** 配置文件包含两个 `[[mounts]]`，type 同为 `"quark"` 但 cookie 不同
- **THEN** 系统成功加载两个实例
- **AND** 每个实例的 name 互不相同

#### Scenario: 字段覆盖默认值
- **WHEN** 配置中 `[defaults.sync]` 设定 `concurrent_uploads = 3`
- **AND** 某个 `[[mounts]]` 的 `[mounts.sync]` 设定 `concurrent_uploads = 5`
- **THEN** 该实例使用 5，其他实例使用 3

### Requirement: 字段合并规则

系统 SHALL 在加载配置后将 defaults 字段合并到每个 mount instance 中，mount-level 字段优先于 global defaults。

#### Scenario: 分层合并 encryption
- **WHEN** `[defaults.encryption]` 设置 password
- **AND** 某 mount 也设置了 `[mounts.encryption]`
- **THEN** 该 mount 使用自己的 encryption 配置
- **AND** 未设置 encryption 的 mount 使用 defaults 的 encryption

#### Scenario: cache 目录独立
- **WHEN** 两个 mount 实例加载
- **THEN** 各自的 cache 目录为 `~/.qrypt/cache/<name>/`
- **AND** 两者互不干扰

### Requirement: name 唯一性校验

系统 SHALL 校验所有 mount name 唯一，并符合 `[a-z0-9-]{1,32}` 格式。

#### Scenario: 重复 name 报错
- **WHEN** 两个 `[[mounts]]` 的 name 相同
- **THEN** 配置加载失败，返回重复 name 错误

#### Scenario: 非法 name 报错
- **WHEN** mount name 包含大写字母或特殊字符
- **THEN** 配置加载失败，返回格式错误
