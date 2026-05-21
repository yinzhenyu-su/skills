## Context

ideal-architecture 重构已完成 daemon 内部所有的组件改进：SessionManager、TransferOrchestrator、CacheInvalidator、thin CLI。但进程模型仍有问题：

- `qryptd` 是独立二进制，必须单独启动，用户多学一个概念
- 8 个 CLI 命令的入口逻辑在重复检查 `IsDaemonRunning()`，不通过则打印 "错误: qryptd 未运行"
- `qryptd` 进程在没有挂载时空转，没有任何作用
- FUSE mount 本来就隐含了一个长驻进程，不需要一个额外的 daemon

核心矛盾：**FUSE 用户不需要管理 daemon（mount 已隐含），CLI 用户被迫管理 daemon。**

当前架构：
```
qryptd 进程 (独立启动)
├── WebSocket Server (~/.qrypt/qryptd.sock)
├── MountManager (管理所有 mount)
├── SessionManager / Orchestrator / CacheInvalidator
├── FUSE loop × N (每个 mount 一个)
└── 信号处理

qrypt CLI → 检查 socket → 连 qryptd → RPC
  (socket 不存在则报错)
```

目标架构：
```
qrypt mount 进程 (mount 即 daemon)
├── FUSE loop
├── WebSocket Server (~/.qrypt/qryptd.sock)
├── MountManager
├── SessionManager / Orchestrator / CacheInvalidator
└── 信号处理

qrypt CLI → 发现 socket → 连接 → RPC
  (socket 不存在 → 自动启动 headless daemon)
```

## Goals / Non-Goals

**Goals:**
- `qrypt mount` 为主入口，单进程包含 FUSE + WS server + 所有 daemon 服务
- CLI 命令自动发现运行中的 mount socket，不再需要显式检查
- 无 mount 运行时 CLI 自动启动 headless daemon（无 FUSE 挂载），零摩擦
- `qryptd` 二进制保留为 `qrypt mount --daemon` 的别名
- 保持 socket 路径 `~/.qrypt/qryptd.sock` 不变，向后兼容

**Non-Goals:**
- 改变 WebSocket IPC 协议（复用现有 JSON-RPC 方法集）
- 改变 MountManager / SessionManager / Orchestrator 内部实现
- 跨盘传输（已有独立变更）
- 删除 `qryptd` 二进制（保持兼容性）

## Decisions

### Decision 1: `qrypt mount` 嵌入 daemon 启动序列

**方案：** `cmd/qrypt/mount.go` 在检测到 daemon 未运行时，不再报错退出，而是直接在当前进程中启动 daemon（包括 WS Server + FUSE）。`cmd/qryptd/main.go` 的启动逻辑（~100 行）整体移入 `qrypt mount`。

```
qrypt mount personal
  → socket 检查: ~/.qrypt/qryptd.sock 不存在
  → load config
  → init: logger, Daemon, WSServer
  → start WS server (goroutine)
  → start FUSE mount (goroutine, cgofuse)
  → wait signal (main goroutine block)
  → graceful shutdown
```

**理由：** FUSE 进程本身就需要 long-running，嵌入 WS server 不增加额外开销。所有 daemon 组件已在 `internal/daemon/` 中，`qrypt mount` 只需要调用它们。不需要新增任何基础设施代码。

**信号处理：** SIGINT/SIGTERM → DaemonShutdown（停 mount + drain orchestrator）→ WS Stop → exit。

### Decision 2: `qryptd` 变为薄包装

**方案：** `cmd/qryptd/main.go` 精简为：
```go
func main() {
    // exec qrypt mount --daemon --all
    // 透传 --config / --log-level / --socket 参数
}
```

**理由：** 向后兼容。用户 muscle memory 的 `qryptd` 仍然可用，但实际调用的是 `qrypt mount --daemon`。

**`--daemon` flag 的含义：** headless 模式，不挂载 FUSE，只启动 WS server + 服务端组件。用于用户只想用 CLI 操作而不需要 FUSE 的场景（或 `--all` 表示挂载所有）。

### Decision 3: CLI 命令 auto-discovery

**方案：** 移除所有 CLI 命令中的 `IsDaemonRunning()` 检查 + "qryptd 未运行" 报错。替换为统一的 `ensureDaemon()` 帮助函数：

```go
func ensureDaemon() (*daemon.WSClient, error) {
    socketPath := daemon.FindSocketPath()
    if daemon.IsDaemonRunning(socketPath) {
        return daemon.DialWS(socketPath)
    }
    // 自动启动 headless daemon
    if err := startDaemonHeadless(); err != nil {
        return nil, fmt.Errorf("启动 daemon 失败: %w", err)
    }
    // 等待 socket 出现
    for i := 0; i < 50; i++ {
        if daemon.IsDaemonRunning(socketPath) {
            return daemon.DialWS(socketPath)
        }
        time.Sleep(10 * time.Millisecond)
    }
    return nil, fmt.Errorf("daemon 未能启动")
}

func startDaemonHeadless() error {
    cfgPath := daemon.FindConfigPath()
    cmd := exec.Command(os.Args[0], "mount", "--daemon")
    cmd.Stdout = nil  // detach
    cmd.Stderr = nil  // detach
    cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
    return cmd.Start()
}
```

**理由：** 对用户来说 CLI 命令总是可用的，不需要理解"daemon"概念。自动启动的 daemon 会在所有 WS 连接关闭后 idle timeout 退出（或用户手动 `qrypt mount --stop-daemon`）。

### Decision 4: headless daemon 生命周期

**方案：** `qrypt mount --daemon` 启动的 headless daemon 在以下条件退出：
1. 收到 SIGINT/SIGTERM
2. 所有 WS 连接断开后 idle timeout（默认 30 秒）
3. 通过 RPC `shutdown` 调用

**理由：** headless daemon 应该在没有客户端使用时自动退出，避免残留进程。30 秒 idle timeout 给用户足够时间运行连续的 CLI 命令（如 `qrypt ls && qrypt push file`）。

### Decision 5: 多 mount 管理

**方案：** 保持当前 `MountManager` 在同一进程中管理所有 mount 的设计。`qrypt mount`（无 `--daemon`）启动所有已启用 mount 的 FUSE；`qrypt mount <name>` 启动指定 mount；`qrypt mount --daemon` 不启动 FUSE。

一个 daemon 进程 = 一个 WS server = 管理所有配置中的 mount。

### Decision 6: 向后兼容

**方案：**
- `~/.qrypt/qryptd.sock` 路径不变，现有客户端工具无需修改
- `qryptd` 二进制保留，作为 `qrypt mount --daemon` 的包装
- CLI 的 `mount_name:path` 语法不变
- `find/cp/config/tool` 逻辑不变（本地工具）

## Risks / Trade-offs

| Risk | Mitigation |
|------|-----------|
| `qrypt mount` 启动慢（加载所有组件）| mount 本来就要加载，无额外开销；`--daemon` 模式跳过 FUSE init 更快 |
| headless daemon 残留 | idle timeout 30 秒自动退出；可 RPC shutdown |
| headless daemon 竞争条件（同时启动多个）| 通过 socket 文件作为锁：首次检查不存在才启动，启动后原子写入 |
| 用户仍手动运行 `qryptd` | `qryptd` 保持可用，输出 deprecation 提示建议改用 `qrypt mount --daemon` |
| 依赖 `os.Exec` 的 headless daemon | 仅在 socket 不存在时触发一次，路径为 `os.Args[0]` 保证相同二进制 |
