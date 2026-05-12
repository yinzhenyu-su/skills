# 同步生命周期：重复编辑与 Staging 清理

## 概述

QryptFS 是异步 FUSE 文件系统。用户编辑文件时，写入内容先存入本地 staging 区域，然后由 worker goroutine 异步排队上传。本文档解释系统如何处理两个关键场景：

1. **同一文件的快速重复编辑**（例如 IDE 每秒自动保存）
2. **Staging 文件生命周期**——创建、所有权转移、清理

---

## 1. 快速重复编辑

### 挑战

同一个 `file.txt` 可能每秒收到多次 `Write → Release → enqueueSync` 循环。没有保护措施时，每个循环都会触发一次独立上传，导致：

- **服务端文件重复**（`file.txt`、`file(1).txt`、`file(2).txt`……）
- **假阳性冲突**（服务端报告"存在同名但不同 fid 的文件"）
- **Staging 文件泄漏**（上传失败或竞态后遗留在磁盘上）
- **编辑丢失**（冷却期间清除了 isDirty）

### 完整链路

```
编辑器                          FUSE                         uploadWorker
  │                              │                              │
  ├─ Write(data) ──────────────► │                              │
  │                    node.isDirty = true                      │
  │                    node.localPath  = staging/<fid>.staging  │
  │                              │                              │
  ├─ Fsync ────────────────────► │                              │
  │                              │                              │
  ├─ Release ──────────────────► │                              │
  │                    enqueueSyncDelay(node, 200ms)             │
  │                    syncQueued = true                        │
  │                              │                              │
  │                              ├───── uploadChan ────────────► │
  │                              │         (200ms 后)           │
  │                              │         syncFile(path, node) │
```

### syncFile 执行流程

```
syncFile(path, node):
│
├─ [提前退出]
│   正在删除的目录？节点已取消？路径已变更？ → 返回
│
├─ 快照（在 node.mu.Lock 保护下）
│   fid, localPath, snapshotName, uploadID, lastPart, ...
│
├─ [10 秒上传冷却]
│   如果 lastUpload 在 10 秒内 → return nil（defer 重新入队）
│
├─ 刷新 n.fid → currentFid
│   （快照中的 fid 在 10 秒延迟后可能已过时）
│
├─ [冲突检测 — 对比服务端父目录]
│   检查 1：文件名匹配，但 fid ≠ currentFid → 冲突
│   检查 2：服务端找不到 currentFid，但文件名已存在 → 冲突
│   （不做 mtime 比较——时钟偏差会产生假阳性）
│
├─ 上传(snapshotName, parentFid, snapshotSize, ...)
│   上传器依次调用：
│     1. deleteExistingFileByName（服务端）
│     2. UploadPre → 获取上传会话
│     3. UploadPart（加密、分片）
│     4. UpdateHash / UploadCommit / UploadFinish
│
├─ 上传后处理（在 node.mu.Lock 保护下）
│   n.fid          = result.Fid
│   n.isDirty      = false
│   n.uploadID     = ""          // 为下次上传清除
│   n.lastPart     = 0
│   staging.Remove(localPath)    // ← 清理
│
├─ [defer 清理]
│   如果 isDirty 仍为 true → 重新入队（syncQueued 保持 true）
│   否则              → syncQueued = false
```

### 如何防止文件重复

```
                        syncFile A (worker 1)          syncFile B (worker 2)
                        ─────────────────────          ─────────────────────
syncQueued=true ──────► 取走任务                        [被阻塞 — syncQueued]

                             ...上传中...

                        defer: isDirty? 是
                        syncQueued 保持 true
                        直接发送到 uploadChan
syncQueued=true ──────►                                 [继续阻塞]
                             ...                           ...

syncFile A 完成 ──────►                                 取走任务

                        ⚠ syncQueued 从未被清除
                        A 的 defer 到 B 取任务之间
                        → Release 看见 syncQueued=true → 跳过
                        → 每个节点同时只有一个活跃任务
```

关键不变量：**`syncQueued` 仅在不重新入队时才设为 `false`**。如果需要重新入队，该标志从第一次 `enqueueSyncDelay` 调用一直到重新入队的直接发送全程保持 `true`。这消除了可能让并发 Release 排入第二个任务的 TOCTOU 窗口。

---

## 2. Staging 文件生命周期

### 状态

```
  [已创建]                  [活跃中]              [已清理]
     │                        │                       ▲
     │  Write/                │  Release               │
     │  Truncate              │  → enqueue             │ 上传
     │  创建                  │  → syncFile            │ 成功
     │  staging 文件          │  → 上传开始            │
     ▼                        ▼                       │
 staging/<fid>.staging ◄─── node.localPath ───────────┘
```

### 状态转换

| 事件 | isDirty | localPath | Staging 文件 |
|------|---------|-----------|-------------|
| 初始（服务端文件同步完成） | false | `""` | — |
| **Write**（无 localPath） | true ← | ⇐ `staging.Create(newFid)` | **已创建** |
| **Write**（已有 localPath） | true ← | 不变 | 写入 |
| **Release** → 入队 | true | 不变 | 仍活跃 |
| **syncFile 快照** | true | 捕获为 `localPath` | 仍活跃 |
| **UploadPre**（上传器中） | true | 不变 | 从中读取 |
| **上传成功** | false → | `n.localPath = ""` → | **已删除** ← |
| 10 秒冷却（重新入队） | true | 不变 | 仍活跃 |
| **新 Write**（无 localPath） | true | `staging.Create(newFid)` | **新 staging** |

### 清理路径

**正常路径（上传成功）：**
```
syncFile → uploader.Upload → 成功
  → n.mu.Lock()
      n.localPath = ""
      staging.Remove(localPath)    ← staging 文件删除
  → n.mu.Unlock()
```

**冲突路径（resolveConflict 被调用）：**
```
syncFile → 检测到冲突 → resolveConflict(path, n, f)
  → 节点重命名为 "[Local Conflict ...]"
  → node.localPath 仍然指向 staging 文件
  → syncFile 返回 nil（从冲突检查提前返回）
  → defer 重新入队
  → 新的 syncFile 为冲突路径执行
  → 上传成功
  → staging.Remove(localPath)    ← 在重试时清理
```

**冲突路径说明：** `resolveConflict` 不会清除 `localPath` 或删除 staging 文件。staging 数据会保留，在冲突文件的重试上传成功时被清理（如果重试耗尽，则在下次 `qrypt push` 时清理）。

**重试耗尽（上传持续失败）：**
```
worker: 重试 1/3 → 失败 → 等待 2s → 重试 2/3 → 失败 → 等待 4s → 重试 3/3 → 失败
  → n.syncQueued = false     （允许将来重试）
  → n.isDirty  = 保持 true   （数据不丢失）
  → 日志 WARN
  → staging 文件保留，用于手动 `qrypt push`
```

### Staging 文件命名

```
Store.Path(fid) → <staging_dir>/<fid>.staging
```

staging 文件以**创建时的 fid** 命名，而非节点的当前 fid。这一点很重要，因为 `n.fid` 在每次上传成功后都会变化（从 `local_<name>_<timestamp>` 变为服务端分配的 fid，下次编辑的 Write 时再次变化）。

上传成功后，`n.localPath` 被清除（`""`）。下次 Write 会用新的 `local_*` fid 创建新的 staging 文件。旧的 staging 文件已经被 `staging.Remove()` 删除。

---

## 3. 关键代码位置

| 文件 | 行号 | 内容 |
|------|------|------|
| `internal/fs/sync.go` | 81-95 | Defer：带 syncQueued 不变量的重新入队 |
| `internal/fs/sync.go` | 109-118 | 锁保护下的快照 |
| `internal/fs/sync.go` | 120-122 | 10 秒上传冷却守卫 |
| `internal/fs/sync.go` | 124-126 | 冷却后重新读取 currentFid |
| `internal/fs/sync.go` | 128-192 | 冲突检测（文件名 + fid 检查） |
| `internal/fs/sync.go` | 221-250 | 上传后处理：fid 更新、清理 |
| `internal/fs/workers.go` | 14-110 | uploadWorker：重试循环、耗尽处理 |
| `internal/fs/write.go` | 77-126 | Write 处理器：staging 创建 |
| `internal/fs/write.go` | 224-249 | Release：enqueueSyncDelay(200ms) |
| `internal/staging/store.go` | 78-91 | Store.Create / Store.Remove |

## 4. 冲突检测决策树

```
文件的 fid 是否以 local_ 开头？
  是 → 跳过全部冲突检查（从未上传过）
  否 → 继续

── 检查 1：文件名冲突 ──────────────────────────
列出服务端父目录。
是否存在解密后文件名相同但 fid 不同的文件？
  是 → resolveConflict → 重命名本地节点 → 以冲突文件上传
  否 → 继续

── 检查 2：我们的 fid 在服务端丢失 ─────────────
服务端是否仍然存在 currentFid 对应的文件？
  是 → 我们的文件存在，无冲突 → 继续上传
  否 → 文件已被远程删除
      → 是否存在同名的其他文件？
          是 → resolveConflict
          否 → 将 fid 重置为 local_（作为新文件上传）

── 上传 ───────────────────────────────────────────
调用 uploader.Upload(snapshotName, parentFid, ...)
```
