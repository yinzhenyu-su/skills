# Core API 重构 Review 发现问题清单

> **Branch:** `refactor-the-core-api`
> **Review 范围:** `core/qrypt/` 新增 + `cmd/qrypt/{ls,cat,mkdir,mv,rm,find,push,pull}.go` 切换 + `cmd/qrypt/{core,platform}.go`
> **构建状态:** `go build ./core/qrypt/` ✅, `go build ./cmd/qrypt/` ✅, `go vet ./...` ✅, `go test ./core/qrypt/` ❌ 无测试

---

## 现状问题

本次重构完成了"接口定义 + 桌面 CLI 切换"，但架构文档验收标准多数未达成：

| 验收指标 | 目标 | 实际 |
|---------|------|------|
| daemon 瘦身 | 移除 ≥900 行 | 0 行（`service.go` 仍 1278 行） |
| core/ 测试覆盖 | 任意平台 `go test` 通过 | `no test files` |
| 移动端平台成本 | ≤500 行胶水 | 桌面端 ~200 行达标，iOS/Android 0 |
| `core/` 不依赖 `daemon/` | ✅ | ✅ |

**核心偏差**：`core/qrypt/` 中的 SessionManager/Orchestrator/EventManager/... 与 `internal/daemon/` 中同名组件是**平行实现，不是迁移**。daemon 中所有文件原封不动，导致代码量翻倍而非缩减。

---

## 修复清单（按优先级）

### P0-1: `pushFile` 进度事件提前发出 + 同步等待缺失

**文件:** `core/qrypt/api.go:474-505`

**问题:**
```go
ok := a.uploadQ.Submit(func(ctx context.Context) error {
    _, putErr := up.Put(ctx, parentFid, encSize, encReader)  // 实际异步上传
    return putErr
})
// ↑ Submit 立即返回，下面 Publish "completed" 立即触发
a.progress.Publish(&ProgressEntry{State: "completed", ...})
return nil
```

- 上传失败被吞掉，调用方收不到错误
- `StatusJSON` 看到 completed 事件但文件可能还在传
- 与原 `daemon/orchestrator.go` 在 `fn` 内部 publish 的语义不一致

**方案:**
- `ProgressHub.Publish` 移入 `Submit` 的闭包内
- 失败时 `State: "failed"` + `Error: err.Error()`
- 同步等待（`wg.Wait()`）或返回 `taskID` 给调用方

---

### P0-2: `pushDirectory` 兄弟目录 `parentFid` 串扰

**文件:** `core/qrypt/api.go:508-599`

**问题:** 闭包共享 `parentFid` 变量，walk 回退到兄弟目录时该变量未重置。

**复现:** `localDir/{a/file1.txt, b/file2.txt}`，`b/file2.txt` 会被上传到 `a/` 下。

**方案:** 用 `path → fid` 映射替换单变量：
```go
dirFidCache := map[string]string{parentPath: parentFid}
// walk 回调内按 local path 查询/创建并缓存
```

---

### P0-3: 决定 `daemon/` 与 `core/` 的关系

**文件:** `internal/daemon/{session,orchestrator,events,progress,ratelimit,cache_invalidator,service}.go`

**问题:** 两套平行实现并存 = 维护成本翻倍，与架构 D5 意图相悖。

**方案（择一）:**

| 方案 | 改动 | 收益 |
|------|------|------|
| **A. 真正迁移** | 把 daemon 中组件改造为依赖 `core/qrypt.Driver` 接口（已定义），删除 daemon/ 中的副本 | 达成架构目标，单一实现 |
| **B. daemon 弃用** | `daemon/service.go` 标记 deprecated，所有 RPC handler 改为调 `core/qrypt.FileAPI` | 保留 WS 协议向后兼容，逐步淘汰 daemon |
| **C. 当前状态** | 维持双实现 | ❌ 不可接受 |

**推荐 B**（向后兼容 + 渐进迁移）。FUSE mount 命令暂不动 `daemon/`（架构明确 FUSE 不走 FileAPI）。

---

### P1-1: `Remove` 的 `recursive` 标志未真正递归删除

**文件:** `core/qrypt/api.go:261-307`

**问题:** `recursive=true` 仅跳过空目录检查，然后直接 `w.Remove(target)`。大多数云盘对非空目录 `Remove` 失败。

**方案:**
- `recursive=false` 且非空 → `ErrNotEmpty`（当前行为保留）
- `recursive=true` → 递归 List → 逐个 Remove 叶子 → Remove 父目录
- 或显式返回 `ErrNotEmpty` 并提示用户用 `rm -r`（CLI 层处理）

---

### P1-2: `Stat` 对根目录/目录自身返回 `ErrNotFound`

**文件:** `core/qrypt/api.go:147-185`

**问题:** `Stat("/")` 时 `parentPath="/"`, `baseName=""`，查不到匹配条目。

**方案:** 当 `path == "/"` 或为目录本身时，特殊路径：
```go
if path == "/" {
    return &FileEntry{ID: rootFid, Name: "/", IsDir: true}, nil
}
```

---

### P1-3: `Move` 缺少 no-clobber 检查 + 跨 mount 语义不清

**文件:** `core/qrypt/api.go:222-259`

**问题:**
- 旧 CLI 有 `--no-clobber` 标志（`cmd/qrypt/mv.go:25`），现 `FileAPI.Move` 签名无此参数
- 目标已存在时无检查，直接 Move/Rename
- 签名 `Move(ctx, mount, oldPath, newPath)` 但只支持单 mount，跨 mount 调用静默失败

**方案:**
```go
// 方案 1: 扩展签名
type MoveOptions struct {
    NoClobber bool
}
func (a *FileAPI) Move(ctx context.Context, mount, oldPath, newPath string, opts MoveOptions) error
```
- `opts.NoClobber` 时检查目标 `parentFid` 下是否已有同名条目
- 跨 mount 调用 → 返回 `ErrPermission` 或拆为 copy+delete

---

### P1-4: `--password` / `--salt` CLI 参数丢失

**文件:** `cmd/qrypt/{ls,cat,mkdir,mv,rm,find,push,pull}.go`

**问题:** 旧版本通过 `cmd.Flags().GetString("password")` 读取 CLI 覆盖，新版全传空字符串 `""`（`core.go:50`）。`newFileAPI` 内 `password, salt` 参数未使用。

**方案:**
- `cmd/qrypt/core.go:25` 的 `newFileAPI` 接收 password/salt 传给 `config.MakeCipher` 覆盖 TOML 值
- 或在 `Options` 结构加 `Password/Salt` 字段由 `FileAPI` 透传
- CLI 命令中 `cmd.Flags().GetString("password")` 仍需保留读取

---

### P2-1: `FileAPI` 缺 `Shutdown()` 方法

**文件:** `core/qrypt/api.go` (新增方法)

**问题:** `Orchestrator` 有 `Shutdown()`（`upload.go:63`）但 `FileAPI` 未暴露。CLI 进程退出时 worker goroutine 不会优雅退出，资源泄漏。

**方案:**
```go
func (a *FileAPI) Shutdown() {
    if a.uploadQ != nil {
        a.uploadQ.Shutdown()
    }
}
```
- CLI 入口 `defer api.Shutdown()`

---

### P2-2: `FileAPI.{sessions,cacheInv}` 字段从未初始化

**文件:** `core/qrypt/api.go:33-37` vs `api.go:60-72`

**问题:** 结构体声明了 `sessions *SessionManager` 和 `cacheInv *CacheInvalidator`，但 `NewFileAPI` 未调用 `NewSessionManager`/`NewCacheInvalidator`。`cmd/qrypt/core.go` 直接注入 Driver，绕过 SessionManager → **架构 D1 验证指标「driver 实例缓存」失效**。

**方案（与 P0-3 联动）:**
- `NewFileAPI` 接受 `DriverFactory` 而非 `Driver`
- `NewFileAPI` 内部 `NewSessionManager(factory)` 并注入到内部操作
- 保留 `a.sessions` 公开访问

---

### P2-3: 缺少单元测试（验收硬指标）

**文件:** `core/qrypt/*_test.go` (待新增)

**问题:** `go test ./core/qrypt/` → `no test files`。架构要求 core/ 可在任意平台 `go test` 通过且仅依赖 mock。

**方案:** 至少覆盖：
- `FileAPI.List/Stat/Mkdir/Move/Remove/Read/Push/Pull/Find` 用 mock Driver
- `SessionManager.Acquire/Release` 并发安全
- `Orchestrator.Submit/Shutdown` worker 生命周期
- `EventManager.Publish/Subscribe` 多订阅者
- `RateLimiter.Wait/WaitFor` 0 / 正常 / 超时
- `Error.Is` Kind 匹配

---

### P2-4: `Mkdir` 错误归类错误

**文件:** `core/qrypt/api.go:215-218`

**问题:** `w.Mkdir` 失败时统一归为 `ErrInternal`，实际可能是 `ErrAlreadyExists`（并发场景）。

**方案:**
```go
_, err = w.Mkdir(...)
if err != nil {
    if isAlreadyExistsErr(err) {
        return NewErrorf(ErrAlreadyExists, "mkdir: %w", err)
    }
    return WrapError(ErrInternal, "mkdir", err)
}
```

---

### P3-1: `NoopDirResolver` 命名与行为不符

**文件:** `core/qrypt/api.go:790-794`

**问题:** 名为 Noop，实际返回 `os.TempDir()` 和 `"."`，不是 noop。

**方案:** 重命名为 `DefaultDirResolver`，或真正 noop（返回 `""`）。

---

### P3-2: `MobileAPI.ctx` 参数静默忽略

**文件:** `core/qrypt/mobile.go:18-63`

**问题:** 签名第一个参数叫 `ctx` 但函数体用 `context.Background()`。gomobile 不支持 `context.Context` 跨语言，所以该参数本就不该存在。

**方案:** 重命名为 `_` 或直接删除参数：
```go
func (m *MobileAPI) List(mount, path string) (string, error)
```

---

### P3-3: `FileAPI.Read` 两次 HTTP RTT

**文件:** `core/qrypt/api.go:344-377`

**问题:** 先 `drv.Read(header)` 再 `drv.Read(body)`，两次 RTT。对 cat 大文件不友好。

**方案:** 接受现状（cat 走临时文件场景下，2 RTT 可接受），或优化为单次读取头部大小然后流式读 body（带 chunked transfer）。

---

## 实施顺序建议

| # | Fix | 文件 | 风险 | 建议 |
|---|-----|------|------|------|
| 1 | `pushFile` 同步等待 + 进度修正 | `api.go:474-505` | 用户体验：失败被吞 | **P0 第一** |
| 2 | `pushDirectory` 兄弟目录串扰 | `api.go:508-599` | 正确性：文件错位 | **P0 第二** |
| 3 | daemon vs core 关系（迁移/弃用） | `internal/daemon/*` | 架构目标 | **P0 第三**（需决策） |
| 4 | `Remove` 递归实现 | `api.go:261-307` | 误删风险 | P1 |
| 5 | `Stat` 根目录处理 | `api.go:147-185` | 边界 bug | P1 |
| 6 | `Move` no-clobber | `api.go:222-259` | CLI 兼容性 | P1 |
| 7 | 恢复 `--password`/`--salt` | `cmd/qrypt/*` | CLI 兼容性 | P1 |
| 8 | `FileAPI.Shutdown()` | `api.go` 新增 | 资源泄漏 | P2 |
| 9 | SessionManager/CacheInvalidator 接线 | `api.go:60-72` | 架构 D5 | P2（与 #3 联动） |
| 10 | 单元测试 | `core/qrypt/*_test.go` | 验收标准 | P2 |
| 11 | `Mkdir` 错误归类 | `api.go:215-218` | 错误处理 | P2 |
| 12 | `NoopDirResolver` 重命名 | `api.go:790-794` | 命名清晰度 | P3 |
| 13 | `MobileAPI.ctx` 参数清理 | `mobile.go` | API 清晰度 | P3 |
| 14 | `FileAPI.Read` RTT 优化 | `api.go:344-377` | 性能 | P3（如必要） |

---

## 测试方案

1. **构建/编译验证:**
   ```bash
   go build ./core/qrypt/
   go build ./cmd/qrypt/
   go vet ./...
   ```

2. **功能回归（CLI 切换正确性）:**
   ```bash
   # 现有 e2e 测试
   go test ./cmd/qrypt/ -run TestE2E
   ```
   - 重点验证 `ls/cat/mkdir/mv/rm/find/push/pull` 在切换后行为不变

3. **新增单元测试（修复 P2-3）:**
   ```bash
   go test ./core/qrypt/ -race
   ```
   - 覆盖所有 FileAPI 方法 + 内部组件
   - 用 mock Driver / Cipher 隔离 I/O

4. **手工回归测试（修复 P0 后必做）:**
   ```bash
   # P0-1 验证
   qrypt push /tmp/large.bin /test  # 故意中断网络，确认错误被返回
   qrypt status                       # 确认 progress 不显示假 completed
   
   # P0-2 验证
   mkdir -p /tmp/qrypt-test/{a,b}
   echo a > /tmp/qrypt-test/a/file.txt
   echo b > /tmp/qrypt-test/b/file.txt
   qrypt push /tmp/qrypt-test /backup   # 检查 /backup/a/file.txt /backup/b/file.txt 位置正确
   ```

5. **daemon vs core 切换（修复 P0-3 后必做）:**
   - FUSE mount 命令验证（`qrypt mount` + `ls mount/` 正常）
   - WS RPC 验证（如果选 B 方案，daemon 的 WebSocket 客户端仍能 ls/cat/push）
