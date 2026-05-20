## MODIFIED Requirements

### Requirement: 驱动选择

系统 SHALL 通过 `mounts[].type` 配置项选择每个挂载实例的存储后端。
系统 SHALL 支持在同一配置文件中声明多个不同驱动类型的挂载实例。

#### Scenario: 选择 Quark 驱动
- **WHEN** `mounts` 数组中某实例的 `type` 设置为 `"quark"`
- **THEN** 该实例使用 Quark 驱动进行所有存储操作

#### Scenario: 同时使用 Quark 和 Yun139
- **WHEN** 声明两个 `[[mounts]]`，其一 `type = "quark"`，其二 `type = "yun139"`
- **THEN** 两个驱动互不干扰，各自独立认证和运行

### Requirement: 驱动参数配置

每种驱动 SHALL 通过 `mounts.params` 节配置独立参数。

#### Scenario: Quark 参数
- **WHEN** `mounts.type = "quark"`
- **THEN** 配置 SHALL 包含 `[mounts.params]` 节
- **AND** 该节 SHALL 支持 `cookie` 和 `root_path` 字段

#### Scenario: 139 参数
- **WHEN** `mounts.type = "yun139"`
- **THEN** 配置 SHALL 包含 `[mounts.params]` 节
- **AND** 该节 SHALL 支持 `authorization` 和 `root_id` 字段

### Requirement: 工厂函数

系统 SHALL 提供一个工厂函数，根据 mount 配置创建对应的驱动实例。

#### Scenario: 工厂函数返回正确驱动
- **WHEN** 传入 `type = "quark"` 的挂载配置
- **THEN** 工厂函数返回 `*drive/quark.QuarkDriver`
- **AND** 驱动已使用配置中的参数初始化

#### Scenario: 多个实例多次调用工厂
- **WHEN** 配置中有 3 个 mount 实例
- **THEN** 工厂函数被调用 3 次
- **AND** 每次返回独立的 Driver 实例

### Requirement: 向后兼容

系统 SHALL 支持旧版 `[drive]` + `[mount]` 顶层配置格式，自动迁移为 `[[mounts]]` 单实例。

#### Scenario: 旧格式自动迁移
- **WHEN** 配置文件使用旧格式（`[drive]` 节，无 `[[mounts]]` 节）
- **THEN** 系统自动创建一个 mount 实例
- **AND** name 从 drive.type 派生（如 `"quark"`）
- **AND** mount_point 从 `[mount].point` 读取
- **AND** params 从对应 `[drive.quark]` 或 `[drive.yun139]` 节迁移
