# Ctrl+C 退出流程修复方案

## 现状问题

Shutdown 关闭 channel 后无等待机制，多个向已关闭 channel 发送的路径靠 `defer recover()` 兜底但不安全（静默丢失 upload）。**最严重的是 FUSE handler 无 panic guard，向关闭的 channel 发送会导致整个进程崩溃（cgofuse 通过 cgo 调用，panic 不被 C 层捕获）**。

详见 [分析文档](.)，按优先级排列修复。

---

## 修复清单（按实施顺序）

### Fix 1 — FUSE handler panic guard + shuttingDown 拒绝

**文件:** `internal/vfs/path_state.go`

**问题:** Unlink/Rmdir handler 直接向 `opsLogChan` / `metadataOpChan` 发送。Shutdown 关闭这些 channel 后，任何残留的 FUSE 操作都会 panic。由于 cgofuse 通过 cgo 调用 Go handler，panic 不被 C 代码捕获→**进程崩溃**。

**方案:**

1. 在 `QryptFS` 上加一个工具方法 `isShuttingDown()`，在 FUSE handler 入口检查：
   ```go
   if fs.isShuttingDown() {
       driver.Log.Warnf("[SHUTDOWN] Rejecting FUSE operation: %s\n", path)
       return -fuse.EIO  // 或 fuse.EACCES / fuse.EBADF
   }
   ```

2. 在所有 FUSE handler（Unlink, Rmdir, Rename, Mkdir, Truncate, Write, Create等）入口处加 `isShuttingDown` 检查，拒绝新操作。

3. 可选：给每个 FUSE handler 加 `defer recover()` 兜底（虽然加了 check 后不应该再触发，但防御性编程）。

---

### Fix 2 — enqueueSyncDelay goroutine 检查 shuttingDown

**文件:** `internal/vfs/sync.go` `enqueueSyncDelay()`

**问题:** 200ms debounce goroutine sleep 醒来后不管 channel 状态直接发送。Ctrl+C 窗口内大量命中，upload 静默丢失（recover 兜底但业务失败）。

**方案:**

```go
if delay > 0 {
    go func() {
        defer func() {
            if r := recover(); r != nil {
                driver.Log.Errorf("PANIC in enqueueSyncDelay: %v\n%s\n", r, debug.Stack())
            }
            // 无论什么原因退出，确保 syncQueued 被清理
        }()
        time.Sleep(delay)
        if atomic.LoadInt32(&fs.shuttingDown) == 1 {
            n.mu.Lock()
            n.syncQueued = false
            n.mu.Unlock()
            return
        }
        fs.uploadChan <- syncTask{node: n}
    }()
}
```

要点：
- 检查 `shuttingDown` 后在 `defer` 中清理 `syncQueued`（不仅 panic recover 时清理）
- 日志级别降为 Warn（正常 shutdown 时不视为错误）

---

### Fix 3 — Retry goroutine TOCTOU 修复

**文件:** `internal/vfs/sync.go` `uploadWorker()` retry goroutine

**问题:** 检查 `shuttingDown == 0` 后、发送前，Shutdown 可能已关闭 channel。

**方案:** 用 `select` 安全发送，或在发送前双检：

```go
// 方式 A: select 试图发送 + 检查 shuttingDown
if atomic.LoadInt32(&fs.shuttingDown) == 1 {
    // cleanup...
    return
}
n.mu.Lock()
n.syncQueued = true
n.mu.Unlock()

select {
case fs.uploadChan <- syncTask{node: n}:
default:
    // channel closed, or full (shouldn't happen with buffer)
    n.mu.Lock()
    n.syncQueued = false
    n.mu.Unlock()
    if atomic.LoadInt32(&fs.shuttingDown) == 1 {
        return // normal during shutdown
    }
}
```

或者更简单的：发送前再检查一次 `shuttingDown`。select 方案更健壮。

---

### Fix 4 — sync.WaitGroup 等待 worker 完成 + 超时

**文件:** `internal/vfs/facade.go` `Shutdown()` + `internal/vfs/types.go` `QryptFS struct`

**问题:** Shutdown 关闭 channel 后立即返回，inflight upload/delete/opslog 得不到等待，`os.Exit(0)` 可能截断进行中的操作。

**方案:**

1. `QryptFS` 新增 `sync.WaitGroup`：
   ```go
   type QryptFS struct {
       // ... existing fields ...
       workerWg sync.WaitGroup
   }
   ```

2. 启动 worker 时计数：
   ```go
   // facade.go NewQryptFS
   for i := 0; i < concurrentUploads; i++ {
       fs.workerWg.Add(1)
       go fs.uploadWorker()
   }
   fs.workerWg.Add(1)
   go fs.metadataWorker()
   fs.workerWg.Add(1)
   go fs.opsLogWorker()
   ```

3. Worker 退出时 Done：
   ```go
   // uploadWorker 退出处
   driver.Log.Info("Upload worker stopped\n")
   fs.workerWg.Done()
   ```

4. Shutdown 时等待（带超时）：
   ```go
   func (fs *QryptFS) Shutdown() {
       atomic.StoreInt32(&fs.shuttingDown, 1)
       close(fs.uploadChan)
       close(fs.metadataOpChan)
       close(fs.opsLogChan)
       
       // Wait for all workers to finish their current tasks
       done := make(chan struct{})
       go func() {
           fs.workerWg.Wait()
           close(done)
       }()
       select {
       case <-done:
           driver.Log.Info("Shutdown: all workers finished\n")
       case <-time.After(30 * time.Second):
           driver.Log.Warnf("Shutdown: timed out waiting for workers (30s)\n")
       }
   }
   ```

**注意:** opsLogWorker 和 metadataWorker 的 inner `select` 在 channel 关闭后有 200/500ms idle timeout，会自然退出。这个 WaitGroup 覆盖所有 worker。

---

### Fix 5 — Ghost file cleanup 防 send-on-closed

**文件:** `internal/vfs/sync.go` `uploadWorker()` 成功路径

**问题:** 上传成功后检测到 ghost file，向 `metadataOpChan` 发送，但可能 channel 已关闭。

**方案:**

```go
if !stillInTree {
    if atomic.LoadInt32(&fs.shuttingDown) == 1 {
        driver.Log.Infof("uploadWorker: skipping ghost file cleanup for %s (shutting down)\n", savedPath)
    } else {
        fs.metadataOpChan <- metadataTask{...}
    }
}
```

---

### Fix 6 — opsLogChan 发送前检查 shuttingDown（Unlink/Rmdir）

**文件:** `internal/vfs/path_state.go`

**问题:** 在 Fix 1 中，FUSE handler 会在 shuttingDown 时拒绝新操作。但如果 shutdown 发生在 handler 执行到一半（已经过了入口检查但尚未发送到 channel），仍然可能命中。

**方案:** 在 `opsLogChan <-` 和 `metadataOpChan <-` 发送前加局部检查：

```go
// path_state.go Unlink/Rmdir
if atomic.LoadInt32(&fs.shuttingDown) == 1 {
    // 已经不需要记录 ops_log 或 delete 了，进程即将退出
    return 0
}
fs.opsLogChan <- task
fs.metadataOpChan <- task
```

**注意:** 正常情况下 Fix 1 的 handler 入口检查应先拦截到，这个作为防御性深度检查。

---

### Fix 7 — os.Exit(0) 前 flulsh log + 替代方案

**文件:** `cmd/qrypt/main.go`

**问题:** `os.Exit(0)` 不执行 defer，`levelLogger.Close()` 不会被调用的日志可能截断尾巴。

**方案:** 信号 handler 退出前显式 flulsh 日志：

```go
// 替换:
levelLogger.Close()  // 显式关闭，不依赖 defer
os.Exit(0)
```

或者改用更优雅的方式（仅在 fusermount 成功时可用）：
```go
// fusermount 成功时
levelLogger.Close()
os.Exit(0)

// fusermount 失败，2s fallback 时
go func() {
    time.Sleep(2 * time.Second)
    levelLogger.Close()
    fmt.Println("强制退出")
    os.Exit(0)
}()
host.Unmount()
```

---

## 实施顺序建议

| # | Fix | 文件 | 风险 | 建议 |
|---|-----|------|------|------|
| 1 | FUSE handler panic guard | path_state.go | 进程 crash | **第一优先级** |
| 2 | enqueueSyncDelay shuttingDown 检查 | sync.go | upload 丢失（macOS 高频） | **第二优先级** |
| 3 | Retry TOCTOU | sync.go | upload 丢失（概率低） | 与 fix 2 一起改 |
| 4 | WaitGroup + Shutdown 超时等待 | facade.go, types.go | inflight 操作被截断 | **第三优先级**（改动较大） |
| 5 | Ghost cleanup guard | sync.go | 服务器幽灵文件 | 附带 fix 2/3 |
| 6 | FUSE handler send 前深度检查 | path_state.go | 进程 crash（防御性） | 附带 fix 1 |
| 7 | os.Exit 前 flulsh log | main.go | 日志截断 | 最后 |

---

## 测试方案

1. **单元测试：** 模拟 `Shutdown()` 后验证 `enqueueSyncDelay`、retry goroutine、FUSE handler 不会 panic
2. **手动测试：** `cp -r dist/ mount/` 进行中按 Ctrl+C，观察日志是否有 `PANIC` 输出
3. **重启验证：** 被截断的 upload 是否被 `recoverDirtyFiles` 正确恢复
