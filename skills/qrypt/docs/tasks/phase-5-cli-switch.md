# Phase 5: 桌面 CLI 切换 (1d)

目标：`cmd/qrypt/` 中的命令改为调用 `core/qrypt.FileAPI`，daemon/ 中的对应方法标记 deprecated。

前置依赖：Phase 3（FileAPI 实现）+ Phase 4（Platform Services 实现）。

## 任务 5.1: NewDaemon 改造

`daemon/service.go:38-56` NewDaemon 当前创建了 SessionManager、EventManager 等组件。
改为引用 core/ 中的实例：

```go
func NewDaemon(cfg *config.Config, version string) *Daemon {
    // daemon 持有 core FileAPI 实例
    fileAPI := newCoreFileAPI(cfg)  // 封装 NewFileAPI
    
    // daemon 仍然需要 MountManager（桌面特有）
    mm := NewMountManager(cfg, fileAPI.Sessions(), fileAPI.Events())
    
    d := &Daemon{
        cfg:       cfg,
        version:   version,
        fileAPI:   fileAPI,    // NEW
        manager:   mm,
        sessionMgr: fileAPI.Sessions(), // 复用 core 的
        eventMgr:  fileAPI.Events(),
        progress:  fileAPI.Progress(),
        rateLimit: fileAPI.RateLimiter(),
    }
    return d
}
```

**注意**：NewDaemon 保持向后兼容，内部逐步替换。

## 任务 5.2: ls.go → FileAPI.List()

`cmd/qrypt/ls.go` (约 100行)：

**Before**: 通过 daemon WS client 调用 `ListDir` RPC（或直接驱动物理路径）。
**After**: `fileAPI.List(ctx, mountName, path)`。

```go
// 关键改动
entries, err := d.FileAPI().List(ctx, mountName, path)
```

## 任务 5.3: cat.go → FileAPI.Read()

`cmd/qrypt/cat.go` (约 80行)：

**Before**: 通过 daemon WS client 调用流式 `CatFile` RPC。
**After**: `fileAPI.Read(ctx, mount, path)` → `io.Copy(os.Stdout, reader)`。

注意：cat 命令需要流式能力（大文件不能全读内存）。FileAPI.Read 返回 `io.ReadCloser`。

## 任务 5.4: push.go → FileAPI.Push()

`cmd/qrypt/push.go` (约 100行)：

**Before**: 通过 daemon WS client 调用 `PushStart` RPC + 监控进度事件。
**After**: `fileAPI.Push(ctx, mount, localPath, remotePath, opts)`。

## 任务 5.5: pull.go → FileAPI.Pull()

`cmd/qrypt/pull.go` (约 100行)：

**Before**: 通过 daemon WS client 调用 `PullStart` RPC。
**After**: `fileAPI.Pull(ctx, mount, remotePath, localPath, opts)`。

## 任务 5.6: mv.go / rm.go / mkdir.go → 对应方法

### mv.go

```go
fileAPI.Move(ctx, mount, srcPath, dstPath)
```

### rm.go

```go
fileAPI.Remove(ctx, mount, path, recursive, force)
```

### mkdir.go

```go
fileAPI.Mkdir(ctx, mount, path)
```

### find.go

```go
fileAPI.Find(ctx, mount, path, pattern, maxDepth, maxMatches, caseSensitive)
```

## 任务 5.7: daemon/service.go deprecated 标记

`daemon/service.go` 中与原数据相关的文件操作方法：
- `ListDir` (L632) → 标记 `@deprecated use core/qrypt.FileAPI`
- `Mkdir` (L804) → 标记 deprecated
- `Remove` (L865) → 标记 deprecated
- `Move` (L940) → 标记 deprecated
- `PushStart` (L315) → 标记 deprecated
- `PullStart` (L1055) → 标记 deprecated

**不移除**：daemon 的 WebSocket API 仍然通过这些方法服务 RPC 客户端。对应的 RPC handler 指向新的 core/ 实现。

## 任务 5.8: WebSocket API 适配

daemon 的 WS API（`daemon/ws_server.go:695`）通过 RPC 调用 Service 接口。
改造方式：

```go
// ws_server.go 中的 RPC 分发
case "list_dir":
    result, err := d.fileAPI.List(ctx, params.MountName, params.Path)
    // 转换为 protocol.ListDirResult 格式
```

**协议不变**，后端实现切换。

## 任务 5.9: config 相关命令保持

`config.go`、`init.go`、`validate.go` 等配置管理命令不需要 FileAPI，保持不变。

`status.go`、`dashboard.go` 等监控命令使用 FileAPI 的 `ProgressHub.Active()` 替代 daemon 的 `ActiveTransfers()`。

## daemon/ 最终瘦身统计

| 文件 | 原行数 | 移除 | 最终行数 |
|------|-------|------|---------|
| `service.go` | 1278 | ~500 (文件操作方法) | ~778 |
| `session.go` | 105 | 105 (全部移走) | ~30 (adapter) |
| `orchestrator.go` | 74 | 74 (全部移走) | ~30 (adapter) |
| `events.go` | 52 | 52 (全部移走) | ~30 (adapter) |
| `progress.go` | 83 | 83 (全部移走) | ~0 (删除) |
| `ratelimit.go` | 44 | 44 (全部移走) | ~0 (删除) |
| `cache_invalidator.go` | 81 | 81 (全部移走) | ~30 (adapter) |
| **总计** | **~1717** | **~939** | **~898** |

## 验证方式

```bash
cd skills/qrypt && go build ./cmd/qrypt/
cd skills/qrypt && go test ./cmd/qrypt/
cd skills/qrypt && go vet ./...
```
CLI 命令的功能测试通过 e2e test。
