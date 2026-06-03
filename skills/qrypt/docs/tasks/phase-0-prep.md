# Phase 0: daemon 职责盘点与边界划定 (0.5d)

目标：确认 daemon/ 中哪些代码移入 core/，哪些保留，无代码改动。

## 任务 0.1: 创建 core/qrypt/ 目录结构

```
qrypt/
└── core/
    └── qrypt/          ← 新建，package qrypt
```

- 在 `go.mod` 中：已有 `github.com/yinzhenyu/skills/qrypt`，`core/qrypt/` 属于同一 module，无需加 replace
- 创建空文件占位，为 Phase 1-3 做准备

## 任务 0.2: 确认移入 core/ 的组件清单

对照 `docs/core-architecture.md` L46 表格，确认以下组件的移入边界：

### 可直接移入（零依赖改动）

| 组件 | 文件 | 行数 | 依赖 |
|------|------|------|------|
| SessionManager | `daemon/session.go` | 105 | `config.MountParams`(值类型) + `drive` + `drive/factory` |
| Orchestrator | `daemon/orchestrator.go` | 74 | `context` + `sync` + `internal/log` |
| RateLimiter | `daemon/ratelimit.go` | 44 | `x/time/rate` |
| EventManager | `daemon/events.go` | 52 | `protocol.Event` |
| ProgressHub | `daemon/progress.go` | 83 | `protocol.Event` + `protocol.PushProgressData` |
| FileAPI 操作 | `daemon/service.go` | ~500 | `drive.Driver` + `crypt.Cipher` + `config` |

### 需先抽象接口才能移入

| 组件 | 依赖问题 | 解决方案 |
|------|---------|---------|
| CacheInvalidator (81行) | 持有 `*MountManager` | 提取 `CacheInvalidatorHooks` 接口（core/qrypt 定义） |

## 任务 0.3: 确认 daemon/ 中保留的代码（不可移入）

以下代码是桌面平台特有的，不移入 core/：

| 文件 | 行数 | 保留原因 |
|------|------|---------|
| `daemon/mount_fuse.go` | 85 | FUSE 挂载生命周期，依赖 cgofuse |
| `daemon/mount_manager.go` | 421 | mount 实例管理，持有 `fs.QryptFS` 等 FUSE 类型 |
| `daemon/ws_server.go` | 695 | Unix socket RPC 服务端 |
| `daemon/ws_client.go` | 172 | Unix socket RPC 客户端 |
| `daemon/socket.go` | 29 | Unix socket 路径工具 |
| `daemon/api.go` | 45 | daemon Service 接口（与 WS 协议绑定） |
| `daemon/mount_nofuse.go` | 31 | 无 FUSE 构建占位 |
| `daemon/dispatch_test.go` | 38 | WS dispatch 测试 |

## 任务 0.4: 梳理 core/ 需要剥离的依赖关系

### 4a. `protocol.Event` → core 自有 Event 类型

EventManager、ProgressHub、CacheInvalidator 都引用了 `internal/protocol.Event`。
core/ 不能依赖 protocol/（因为 protocol/ 是桌面 IPC 协议定义）。

方案：在 `core/qrypt/events.go` 中定义精简 Event：

```go
type EventType string

const (
    EventSyncProgress  EventType = "sync_progress"
    EventSyncCompleted EventType = "sync_completed"
    EventSyncFailed    EventType = "sync_failed"
)

type Event struct {
    Type      EventType
    Mount     string
    Timestamp int64
    Data      interface{}
}
```

daemon/ 中保留对 `protocol.Event` 的适配层。

### 4b. `config.MountParams` → 值类型参数

SessionManager 当前接收 `config.MountParams`。core/ 不能依赖 config/。
方案：将 `SessionKey` 和 `params` 改为核心自有的值类型，`WithMountParams` 或直接传 `map[string]string`。

**拆解分析**：`config.MountParams` 结构：
```go
type MountParams struct {
    Cookie        string
    RootPath      string
    Authorization string
    RootID        string
    LocalRoot     string
}
```
core 只需要的是 `Type`（驱动类型）和认证凭据。可在 core/ 中定义：
```go
type SessionConfig struct {
    Type   string
    Params map[string]string
}
```
具体驱动工厂适配在各自平台实现中完成。

### 4c. `drive.Driver` 接口 → core 自建精简接口

core/ 不直接引用 `internal/drive.Driver`。接口定义在 Phase 1 完成。

### 4d. `internal/log` → 平台无关日志接口

Orchestrator 依赖 `internal/log.L.Errorf`。
方案：在 core/ 定义 `Logger` 接口，Platform Services 中注入实现。
或 Orchestrator 直接返回 error，由上层处理日志。

## 任务 0.5: 确认验收标准

- `core/` 不依赖 `internal/daemon/`、`internal/fs/`、`internal/protocol/`、`internal/config/`
- `go build ./core/qrypt/` 在 macOS + Linux 通过
- `internal/daemon/` 移除 ≥900 行后编译通过
