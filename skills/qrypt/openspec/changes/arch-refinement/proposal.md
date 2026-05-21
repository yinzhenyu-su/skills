## Why

mount-as-daemon 重构已解决进程模型问题，但架构内还有 3 个结构性遗留问题未解决：下载路径未统一、find 绕过 daemon、函数命名过时。

## What Changes

- **下载统一到 Orchestrator** — `Daemon.PullStart()` 从 `sync.WorkerPool` 迁移到 `TransferOrchestrator`，统一限速/进度/并发。`internal/sync/pool.go` WorkerPool 退役删除。
- **find 改为 daemon RPC** — 新增 `find` RPC method，daemon 端递归 walk 实现搜索。`find.go` 使用 `ensureDaemon()` + RPC，高级过滤（glob/regex/size/type）在 CLI 端二次过滤。
- **runMountViaDaemon 改名** — 改为 `delegateMountToRunningDaemon`。

## Impact

| 包 | 变化 |
|----|------|
| `internal/daemon/orchestrator.go` | 扩展支持下载任务 |
| `internal/daemon/service.go` | PullStart 改用 Orchestrator |
| `internal/sync/pool.go` | 删除 WorkerPool |
| `internal/daemon/ws_server.go` | 新增 `find` RPC method |
| `cmd/qrypt/find.go` | 移除本地 driver 创建，改走 daemon RPC |
| `cmd/qrypt/mount.go` | runMountViaDaemon 改名 |
