## Why

当前 qrypt 只支持单个网盘实例挂载（一个 cookie、一个 type、一个 mount point）。用户需要同时挂载多个夸克账户、或多个不同类型的网盘（夸克 + 天翼云 + 本地文件系统），并且在同一进程中管理它们。分离的多个 `qrypt mount` 进程无法实现跨盘传输、无法统一管理配置和状态。

## What Changes

- **配置模型**：从单 `[drive]` + `[mount]` 改为 `[[mounts]]` 数组，每个实例独立声明 type、params、mount_point、encryption
- **配置向后兼容**：旧格式 `[drive]` + `[mount]` 自动迁移到 `[[mounts]]` 单实例
- **MountManager**：新增 `internal/daemon/mount_manager.go`，管理多个 `MountInstance` 的生命周期（start/stop/status）
- **qryptd 改造**：守护进程启动时自动挂载所有 enabled 实例
- **CLI 扩展**：`qrypt mount list/start/stop` 子命令；工具命令支持 `mount_name:path` 前缀语法
- **跨盘传输**：`qrypt cp` 支持在 mount 之间流式传输，不经本地磁盘中转
- **Unix socket API 扩展**：所有 RPC method 支持 `mount` 参数作用域

## Capabilities

### New Capabilities

- `multi-mount-config`: 多实例 TOML 配置模型，含向后兼容迁移和字段合并规则
- `mount-manager`: 多挂载实例生命周期管理（start/stop/status 及独立 cipher/cache/driver）
- `mount-name-path`: CLI 中 `mount_name:path` 语法解析及消歧规则
- `cross-drive-transfer`: 跨 mount 流式文件传输管道（解密 → 重加密 → 上传）

### Modified Capabilities

- `driver-config`: requirements 从单实例改为支持数组；新增 DefaultsConfig 概念
- `drive-abstraction`: 无 requirement 变更（Driver 接口本身不需要改，multi-mount 是配置层问题）

## Impact

- `internal/config/` — 新增 `MountInstance`, `DefaultsConfig`, `MountParams` 类型；`LoadConfig` 加迁移逻辑；`ValidateConfig` 循环校验
- `internal/daemon/` — 新增 `mount_manager.go`；`Daemon` 重构为调用 `MountManager`；`server.go` dispatch 加 mount 路由
- `internal/protocol/` — 新增多实例 status 结构
- `internal/transfer/` — 新增跨盘传输模块
- `cmd/qrypt/` — 新增 `mount` 子命令；`util.go` 加 `ParseMountPath()`
- `cmd/qryptd/` — 改造为启动所有 enabled mount
- `openspec/specs/driver-config/spec.md` — 更新 requirements 支持多实例
