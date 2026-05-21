## ADDED Requirements

### Requirement: MountManager 管理多个挂载实例

系统 SHALL 提供 MountManager 组件，管理多个 MountInstance 的生命周期。

#### Scenario: 启动单个 mount
- **WHEN** 调用 `MountManager.Start(ctx, "personal")`
- **THEN** 系统找到名为 personal 的 resolved config
- **AND** 创建对应的 cipher、driver、cacheManager、QryptFS
- **AND** 完成 FUSE 挂载
- **AND** mount 状态变为 mounted

#### Scenario: 停止单个 mount
- **WHEN** 调用 `MountManager.Stop(ctx, "personal")`
- **THEN** 系统卸载 FUSE 挂载
- **AND** 关闭 cache manager
- **AND** 调用 driver.Drop()
- **AND** mount 状态变为 unmounted

#### Scenario: 启动所有 enabled mount
- **WHEN** 调用 `MountManager.StartAll(ctx)`
- **THEN** 遍历所有 mount，对 enabled=true 的执行 Start
- **AND** 某个 mount 失败不影响其他 mount 继续启动
- **AND** 失败的 mount 状态为 error，LastError 记录原因

### Requirement: 状态查询

系统 SHALL 提供查询所有 mount 状态的能力。

#### Scenario: 列出所有 mount
- **WHEN** 调用 `MountManager.List()`
- **THEN** 返回所有 mount 的 name、state、mount_point、drive_type、uptime、last_error

#### Scenario: 按路径反查 mount
- **WHEN** 调用 `MountManager.LookupByPath("/Users/user/Qrypt/Personal/docs/file.txt")`
- **THEN** 返回对应的 MountInstance 和相对路径 `docs/file.txt`
- **AND** 当路径不匹配任何 mount_point 时返回 error
