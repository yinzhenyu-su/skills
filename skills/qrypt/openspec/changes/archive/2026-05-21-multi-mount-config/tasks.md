## 1. Config 层 — 新数据结构与迁移

- [x] 1.1 在 `internal/config/config.go` 新增 `MountInstance`、`DefaultsConfig`、`MountParams`、`ResolvedMountConfig` 结构体
- [x] 1.2 `LoadConfig` 新增向后兼容迁移逻辑：旧 `[drive]` + `[mount]` → 自动生成 `[[mounts]]` 单实例
- [x] 1.3 实现 `MergeInstanceConfig()` 函数，将 defaults 合并到每个 mount instance
- [x] 1.4 `ValidateConfig` 改为循环校验 `[]MountInstance` — name 唯一性、type 合法性、必填字段
- [x] 1.5 `RootPath()` + 新增 `RootPathForMount()` 支持按 mount 查询
- [x] 1.6 更新 `WriteDefaultConfig()` 输出新格式 `[[mounts]]`
- [x] 1.7 单元测试覆盖：旧格式迁移、多实例、字段合并、校验失败场景

## 2. MountManager — 多实例生命周期管理

- [x] 2.1 在 `internal/daemon/mount_manager.go` 实现 `MountManager` 结构体（map[name]*MountInstance）
- [x] 2.2 实现 `MountManager.Start(ctx, name)` — cipher/driver/cache/QryptFS 创建链路
- [x] 2.3 实现 `MountManager.Stop(ctx, name)` — 卸载 FUSE + 关闭 cache + driver.Drop
- [x] 2.4 实现 `MountManager.StartAll()` / `StopAll()` — 逐个启动/停止，失败不阻断
- [x] 2.5 实现 `MountManager.List()` / `Get()` / `LookupByPath()` 查询接口
- [x] 2.6 `mount_fuse.go` 的 `mount()` 函数改为接受 `ResolvedMountConfig`
- [x] 2.7 单元测试覆盖：单实例启停、多实例隔离、LookupByPath

## 3. qryptd 改造 — 守护进程支持多 mount

- [x] 3.1 `Daemon` 结构体重构：持有 `MountManager`，`Start/Stop` 委派给 Manager
- [x] 3.2 `server.go` dispatch 新增 `mount_list` / `mount_start` / `mount_stop` / `mount_start_all` RPC
- [ ] 3.3 `cmd/qryptd/main.go` 启动时调用 `MountManager.StartAll(ctx)`
- [x] 3.4 `protocol/types.go` 新增 `MountSummary` 结构体（多实例状态）
- [ ] 3.5 集成测试：qryptd 多 mount 启停、状态查询

## 4. CLI — mount 子命令与 path 语法

- [x] 4.1 在 `cmd/qrypt/mount_admin.go` 新增 `mount list/start/stop` 子命令
- [x] 4.2 在 `cmd/qrypt/util.go` 实现 `ParseMountPath()` — `mount_name:path` 语法解析
- [x] 4.3 在 `loadToolDriverForMount()` 中加入 `--mount` 参数支持，从配置中选取特定 mount 的 driver
- [x] 4.4 CLI 工具命令（ls/cat/pull/push/rm/mv）支持 `mount_name:path` 语法 + `--mount` 参数
- [x] 4.5 单 mount 模式的向后兼容：无前缀时自动匹配唯一实例
- [x] 4.6 更新 `.qrypt.example.toml` 为新格式

## 5. 跨盘传输 — TransferManager

- [x] 5.1 在 `internal/transfer/` 实现 `TransferManager` — 持有 `MountManager` 引用
- [x] 5.2 实现 `Copy()` 骨架 — 校验 src/dst 是否运行中
- [ ] 5.3 解密 → 重加密管道（待完成：需要 QryptFS 的 file nonce 上下文）
- [x] 5.4 在 `cmd/qrypt/` 新增 `cp` 子命令
- [ ] 5.5 单元测试（待完成：需要 mock driver 支持加密管道）

## 6. 回归与清理

- [x] 6.1 现有单元测试全部通过（`go test ./...`）
- [x] 6.2 旧格式配置 `[drive]` + `[mount]` 自动迁移已验证（单元测试覆盖）
- [x] 6.3 单 mount 模式无行为变化（config 层向后兼容 + Daemon 委派给 MountManager 保持接口）
- [x] 6.4 清理 deprecated 字段相关代码（保留兼容逻辑，标记 deprecated 注释）
