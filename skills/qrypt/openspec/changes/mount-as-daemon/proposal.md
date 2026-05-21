## Why

ideal-architecture 已解决组件层面的问题（SessionManager、TransferOrchestrator、CacheInvalidator、thin CLI），但进程模型仍然不合理：qryptd 是一个需要独立启动和管理的守护进程，8 个 CLI 命令每次都检查 "qryptd 未运行"。

FUSE 是用户最长时间打交道的组件，FUSE 进程本身就该是 daemon。不需要一个额外的进程概念。

## What Changes

- **`qrypt mount` 成为主进程** — 启动 FUSE + WebSocket IPC server + 所有 daemon 服务（SessionManager、Orchestrator、CacheInvalidator），单进程运行直到 unmount。不再需要独立的 qryptd。
- **`qryptd` 二进制保留但变薄** — 作为 `qrypt mount --daemon` 的别名（headless 模式，无 FUSE 挂载），用于纯 CLI 场景。
- **CLI 自动发现 mount socket** — push/pull/ls/cat/rm/mv/mkdir 不再检查 `IsDaemonRunning()`，改为扫描 `~/.qrypt/*.sock` 发现运行中的 mount；`mount_name:path` 语法按名连对应的 socket。
- **无 daemon 时的自动启动** — 如果 socket 扫描发现没有运行中的 mount，CLI 自动启动一个 headless daemon 执行命令，完成后退出。
- **移除了 8 个命令中的 "qryptd 未运行" 检查**。
- **Breaking**: 每个 mount 独立进程（不再共享 Session 和 Orchestrator），但同账号多 mount 场景极少。

## Capabilities

### New Capabilities
- `mount-socket-discovery`: 通过扫描 `~/.qrypt/*.sock` 发现运行中的 mount 实例，支持按 mount name 精确寻址
- `auto-daemon`: CLI 在没有运行中 mount 时自动启动 headless daemon，实现零摩擦的 CLI 体验

### Modified Capabilities
<!-- None — implementation detail, not a spec-level behavior change -->

## Impact

| 包 | 变化 |
|----|------|
| `cmd/qrypt/mount.go` | 从 RPC client 变为主入口：嵌入 FUSE + WS server + daemon 组件；新增 `--daemon` flag 用于 headless 模式 |
| `cmd/qrypt/mount_admin.go` | 精简：mount list/start/stop 改为直接操作管理本地 mount socket |
| `cmd/qrypt/ls.go` | 移除 `IsDaemonRunning` 检查，替换为 socket discovery |
| `cmd/qrypt/cat.go` | 同上 |
| `cmd/qrypt/push.go` | 同上 |
| `cmd/qrypt/pull.go` | 同上 |
| `cmd/qrypt/rm.go` | 同上 |
| `cmd/qrypt/mv.go` | 同上 |
| `cmd/qrypt/mkdir.go` | 同上 |
| `cmd/qryptd/main.go` | 变薄：调用 `qrypt mount --daemon` 的别名 |
| `internal/daemon/wsserver.go` | 启动/停止逻辑调整为 mount 进程生命周期 |
| `internal/daemon/socket.go` | 新增 socket 发现机制、socket 文件注册/清理 |
