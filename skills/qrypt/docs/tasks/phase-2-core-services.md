# Phase 2: 实现 core/qrypt/ Core Services (1d)

目标：将 daemon/ 中的平台无关组件迁移到 core/qrypt/，剥离对 internal/ 包的依赖。

前置依赖：Phase 1 完成（接口定义已就位）。

## 任务 2.1: SessionManager (daemon/session.go → core/qrypt/session.go)

### 源文件分析

`daemon/session.go` (105行)：
- 依赖：`config.MountParams`、`drive`、`drive/factory`
- 功能：引用计数的 driver 连接池

### 迁移方案

**改动点**：

1. `SessionKey` 保持不变，`CredKey` 保持 `string`
2. `SessionConfig` 替代 `config.MountParams`：
   ```go
   type SessionConfig struct {
       Type   string
       Cookie string
       Auth   string
       RootID string
   }
   ```
3. `factory.NewDriverFromType` → 注入 `DriverFactory` 接口：
   ```go
   type DriverFactory interface {
       CreateDriver(ctx context.Context, cfg SessionConfig) (Driver, error)
   }
   ```
4. `config.ResolvedMountConfig` → 使用 `SessionConfig`

**保留在原处**：
- `SessionKeyForMount`（依赖 config/）→ daemon/ 中保留，返回 `SessionKey` 后传给 core/

### 迁移后 daemon/ 改动

`daemon/session.go` 删除 105 行，替换为：
```go
// adapter: 将 core/qrypt.Session 转换为 daemon 兼容形式
```
或直接包装 `core/qrypt.SessionManager`。

## 任务 2.2: UploadQueue/Orchestrator (daemon/orchestrator.go → core/qrypt/upload.go)

### 源文件分析

`daemon/orchestrator.go` (74行)：
- 依赖：`context`、`sync`、`internal/log`
- 功能：worker pool，实现 `fs.UploadQueue`

### 迁移方案

**改动点**：

1. `internal/log` → 注入 `Logger` 接口（或在 core 中定义）：
   ```go
   type Logger interface {
       Errorf(format string, args ...interface{})
   }
   ```
   或直接去掉日志：让调用方处理错误。
   **推荐**：去掉错误日志，改为返回值。orchestrator 不负责记录错误，上层决定如何处理。

2. `Orchestrator` 中的 `tokenBucket *RateLimiter` → 使用 `core/qrypt.RateLimiter`

3. UploadQueue 接口定义从 `internal/fs/fs.go:35` 迁入 core/qrypt/（去掉 build tag）

**核心逻辑不变**：worker pool + channel 任务分发。

## 任务 2.3: EventManager (daemon/events.go → core/qrypt/events.go)

### 源文件分析

`daemon/events.go` (52行)：
- 依赖：`sync`、`protocol.Event`
- 功能：纯 Go pub/sub

### 迁移方案

**改动点**：

1. `protocol.Event` → `core/qrypt.Event`（在 core/qrypt/events.go 中定义）
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

2. `protocol.EventSyncProgress` 等常量 → core 自有常量

**无需改动**：订阅/取消订阅/发布逻辑完全不变。

### daemon/ 适配

daemon/ 中的 EventManager 包装 core/qrypt.EventManager，将 `protocol.Event` 转发到 WebSocket：
```go
type DaemonEventManager struct {
    inner *qrypt.EventManager
}
```

## 任务 2.4: ProgressHub (daemon/progress.go → core/qrypt/progress.go)

### 源文件分析

`daemon/progress.go` (83行)：
- 依赖：`sync`、`time`、`protocol.Event`、`protocol.PushProgressData`
- 功能：收集传输进度，发布事件

### 迁移方案

**改动点**：

1. `protocol.PushProgressData` → core 自有的 `ProgressEntry`（保持现有字段）
2. `protocol.Event` → `core/qrypt.Event`
3. `protocol.EventSyncProgress/Completed/Failed` → core 自有常量

**核心逻辑不变**：`Publish` 方法记录进度 + 发布事件。

## 任务 2.5: RateLimiter (daemon/ratelimit.go → core/qrypt/ratelimit.go)

### 源文件分析

`daemon/ratelimit.go` (44行)：
- 依赖：`context`、`time`、`x/time/rate`
- 功能：包装 `golang.org/x/time/rate`

### 迁移方案

**零改动**，直接复制。依赖已经是标准库 + 已有 go.mod 依赖。

## 任务 2.6: CacheInvalidator (daemon/cache_invalidator.go → core/qrypt/cache.go)

### 源文件分析

`daemon/cache_invalidator.go` (81行)：
- 依赖：`context`、`encoding/json`、`path/filepath`、`internal/log`、`internal/protocol`
- 功能：监听传输完成事件，使目录缓存失效

### 迁移方案

**核心问题**：`CacheInvalidator.manager *MountManager` 是 daemon 特有的类型。

架构文档 D5 (L352-367) 的方案：

```go
// core/qrypt/cache.go

type CacheInvalidatorHooks interface {
    InvalidateDirCache(mountName, dirPath string) error
}

type CacheInvalidator struct {
    hooks CacheInvalidatorHooks
    subID string
}
```

daemon/ 实现 `CacheInvalidatorHooks`：
```go
// daemon/cache_invalidator_adapter.go
type daemonCacheHooks struct {
    manager *MountManager
}

func (h *daemonCacheHooks) InvalidateDirCache(mountName, dirPath string) error {
    inst, err := h.manager.Get(mountName)
    if err != nil {
        return err
    }
    if inst.Backend != nil && inst.Backend.VFS() != nil {
        inst.Backend.VFS().InvalidateDirCache(dirPath)
    }
    return nil
}
```

**核心逻辑不变**：订阅事件 → 解析 progress → 调用 hooks 失效缓存。

## 迁移后 daemon/ 的变更

| daemon/ 文件 | 变更 |
|-------------|------|
| `session.go` | 删除 105 行，改为包装 core/qrypt.SessionManager |
| `orchestrator.go` | 删除 74 行，改为包装 core/qrypt.Orchestrator |
| `events.go` | 删除 52 行，改为包装 core/qrypt.EventManager |
| `progress.go` | 删除 83 行，改为包装 core/qrypt.ProgressHub |
| `ratelimit.go` | 删除 44 行，改为包装 core/qrypt.RateLimiter |
| `cache_invalidator.go` | 删除 81 行，改为 CoreInvalidator + daemonCacheHooks |
| `service.go:38-56` | NewDaemon 改为引用 core/ 中的组件 |
| **总计删除** | **~440 行** |

## 关于 internal/log 的依赖

Orchestrator 和 CacheInvalidator 使用了 `log.L.Errorf`。
core/ 中**不引入日志接口**，改为：
- Orchestrator: 去掉错误日志，返回 error 给调用方
- CacheInvalidator: 去掉调试日志，或使用注入的 Logger 接口（可选）

## 验证方式

```bash
cd skills/qrypt && go build ./core/qrypt/
cd skills/qrypt && go vet ./core/qrypt/
cd skills/qrypt && go test ./core/qrypt/
cd skills/qrypt && go build ./cmd/qrypt/
cd skills/qrypt && go test ./...
```
