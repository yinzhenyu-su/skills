# macOS FUSE Release-before-Write 0 字节上传 — 完整修复历程

## 日期
2026-05-08 ~ 2026-05-09

## 问题

批量上传 dist 目录时（`cp -r dist/ /mnt/qrypt/`），前几个文件（version.json, index.html, favicon.ico）在网盘中显示为 0 字节。

## 根因

macOS FUSE 内核在 `Write` 完成之前就调用了 `Release`。`Release` 触发 `enqueueSync` → `syncFile`，此时 staging 文件为空（`n.size=0, stagingFileSize=0`），`syncFile` 用 `PlainSize=0` 调用 `Sync` → `UploadPre` 创建 32 字节 placeholder（加密空文件）→ 服务器上文件为 0 字节。

## 修复历程（3 次迭代）

### 第 1 次：空 staging 硬跳过（commit `eeef62f`）

```go
if n.size == 0 && stagingFileSize == 0 {
    n.syncQueued = false
    n.mu.Unlock()
    return nil // 跳过
}
```

**结果：** macOS 上传 0 字节修复，但空文件（`touch file`）死循环——skip 后 `defer` 检测 `isStillDirty=true` → re-enqueue → 再次 skip → 无限循环。

**原因：** 空文件永远不会有 Write 到来，staging 永远为空，skip 永远触发，defer 永远 re-enqueue。

### 第 2 次：移除跳过 + 100ms sleep（commit `e1779bc` + `b881743`）

移除硬跳过解决空文件死循环。加 100ms sleep 区分两种场景：

```go
if n.size == 0 && n.localPath != "" && fs.staging != nil {
    time.Sleep(100 * time.Millisecond)
    if actualSize, err := fs.staging.FileSize(n.localPath); err == nil && actualSize > 0 {
        n.size = actualSize
        n.mu.Unlock()
        return nil // macOS FUSE：Write 到了，跳过
    }
    // 空文件：staging 仍空，继续上传 0 字节
}
```

**结果：** 0 字节问题依然存在。

**原因 1（持锁 sleep）：** sleep 在 `n.mu.Lock()` 持锁期间执行，`Write` 需要 `node.mu.Lock()` 写 staging → 被阻塞 → sleep 期间 Write 无法写入 → sleep 形同虚设。

**修复（commit `1d1d35c`）：** sleep 前 Unlock、sleep 后 Lock。但 100ms 仍然不够——macOS FUSE Release→Write 延迟超过 100ms，sleep 结束时 Write 还没到。

**日志证据：**
```
09:25:46  Create /dist/version.json
09:25:46  syncFile DEBUG snapshotSize=0 stagingFileSize=0   ← 100ms 已结束，staging 仍空
09:25:46  deleteExistingFileByName...                       ← syncFile 继续上传
09:25:46  Write /dist/version.json, len=38                  ← Write 在上传开始后才到
09:25:46  UploadPre result: plainSize=0 encSize=32           ← 0 字节上传完成
```

### 第 3 次：enqueueSync 200ms debounce（commit `99b961e`）

**核心洞察：** sleep 在 syncFile 里不可靠，因为 syncFile 是从 channel 异步消费的，sleep 起点不确定。正确的 debounce 位置是 **Release → enqueueSync 这一步**。

```go
// writeback.go — Release
fs.enqueueSyncDelay(node, 200*time.Millisecond)

// sync.go — enqueueSyncDelay
func (fs *QryptFS) enqueueSyncDelay(n *node, delay time.Duration) {
    // ... checks ...
    n.syncQueued = true
    n.mu.Unlock()
    if delay > 0 {
        go func() {
            time.Sleep(delay)
            fs.uploadChan <- syncTask{node: n}
        }()
    } else {
        fs.uploadChan <- syncTask{node: n}
    }
}
```

**时序：**
```
Release → enqueueSyncDelay → 启动 200ms goroutine → Release 返回
                              ↓ 200ms 内 Write 到达，写入 staging，n.size 更新
                              ↓ 200ms 后投递任务到 channel
uploadWorker → syncFile → n.size > 0 → 直接上传真实数据
```

同时移除 syncFile 内的 100ms sleep block——debounce 已保证时序，sleep 对空文件是无谓的延迟。

## 三种场景验证

| 场景 | 行为 | 结果 |
|------|------|------|
| macOS Release-before-Write | 200ms debounce → Write 到达 → syncFile 看到 n.size > 0 → 上传真实数据 | ✅ |
| 空文件 `touch` | 200ms debounce → 无 Write → syncFile 看到 n.size = 0 → 上传 0 字节 → isDirty=false → defer 不 re-enqueue | ✅ 无死循环 |
| 正常文件（Write 在 Release 前完成）| Release 时 n.size 已 > 0 → debounce 后 syncFile 正常上传 | ✅ |

## (1) 后缀问题

不会出现：debounce 确保 syncFile 看到真实数据，无 0 字节误上传 → 无重复文件。

## 关键教训

1. **debounce 位置很重要**：在调用方（Release）做 debounce 比在消费方（syncFile）做 sleep 更可靠，因为消费方的执行时机不确定
2. **持锁 sleep 是反模式**：sleep 期间持有 mutex 会阻塞其他 goroutine 获取同一把锁，使 sleep 的等待目的落空
3. **空文件和 macOS Release-before-Write 的区分**：不能用"跳过"来区分（会导致空文件死循环），只能用"等待"来区分（给 Write 时间到达）
4. **defer re-enqueue 不受 debounce 影响**：`enqueueSync`（delay=0）用于 defer 补发，`enqueueSyncDelay`（delay=200ms）仅用于 Release 首次触发

## 涉及文件

- `internal/vfs/sync.go` — enqueueSyncDelay 实现 + 移除 syncFile 内 100ms sleep
- `internal/vfs/writeback.go` — Release 调用 enqueueSyncDelay

## 相关文档

- `docs/done/2026-05-08-macos-release-before-write-0byte-fix.md` — 初始问题描述（已被本文档取代）
- `docs/done/writeback_delay_optimization.md` — 早期 debounce 设计方案（已被 Release-only sync 取代）
