## Context

qrypt 的 FUSE Write 处理器目前直接调用 `staging.WriteAt()`，每次 Write（编辑器常见 8K/16K 分片）都会触发一次 open+write+close 磁盘 IO。大文件保存时编辑器通常在 1-2 秒内发出数十次 Write，导致 staging 磁盘写入放大严重。

Quark API 层面：批量删除（rm）当前是循环单次调用，ListFiles 同一目录在 15 秒内仍可能重复查询。

同步链路存在两处小竞态：`expectedFid` 在重新编辑后未清除；进程 crash 后 staging 残留无清理。

## Goals / Non-Goals

**Goals:**
- 连续 Write 到同一 staging 文件合并为更少的磁盘写入（目标：大文件保存时 IO 减少 10x+）
- 批量删除改为单次 API 请求
- 消除 `expectedFid` 竞态窗口
- 启动时自动清理孤儿 staging 文件
- 保持 `fsync` / `Release` 的同步语义（用户保存时数据必须落盘）

**Non-Goals:**
- 不上缓存层（仍直接 WriteAt，仅延迟合并同一文件的写入）
- 不改 Upload 链路（uploader 照常从 staging 读取已落盘数据）
- 不做 WebDAV 网关或其他后端适配
- 不改加密逻辑

## Decisions

### D1: Staging write buffer — 定时 flush + 事件触发

```
Write(data, offset)
  ↓
buffer.WriteAt(data, offset)   ← 内存 Page（per staging fd）
  ↓
timer reset (250ms)            ← 同一 staging 文件每次 Write 重置定时器
  ↓
timer fire → flush() → staging.WriteAt(page.Data, page.Off)
  ↓
buffer.Clear()
```

关键细节：

1. **Buffer 粒度：按 fid 分 Page**。每个活跃的 staging 文件一个 `Page` 结构体，内含 `[]byte` 和脏标记。
2. **Flush 触发时机**：
   - 定时器到期（250ms，可配置）
   - `Fsync` / `Release` 时强制同步（同步语义保证）
   - 关闭/重新打开文件时强制 flush
   - Page 满（>1MB）时强制 flush
3. **竞态保护**：`Page` 读写用 `sync.Mutex`；Write 处理器和 Flush goroutine 之间通过 channel 交互。
4. **兼容性**：`staging.WriteAt` 签名不变，行为变为"如果 page 命中则缓冲，否则直写"。对外层代码透明。

与现有流程的关系：

```
当前:      Write → staging.WriteAt(fd, data, off) → open+write+close
改造后:    Write → staging.WriteAt(fd, data, off) → page.WriteAt(data, off) [+reset timer]
            Release → staging.Sync(fd) → flush page → disk WriteAt
```

### D2: API 请求合并 — 批量操作

**批量删除**：`internal/quark/api_manage.go` 新增 `BatchDelete(fids []string)`，检查 Quark API 当前是否支持批量删除（已有的 `/file/delete` 端点支持 `action_type=1&filelist=[fid1,fid2,...]`）。`internal/fs/delete.go` 的 `rm` 路径改为收集 fid 后一次调用。

**批量重命名**：优先级低。当前 `mv` 操作不密集，暂不优化。

### D3: expectedFid 清除 — 2 行改动

`internal/fs/write.go`：Write 处理器中，当 `node.localPath == ""` 创建新 staging 时（即文件被重新编辑），同时清除 `node.expectedFid = ""`。

### D4: Staging crash 清理 — 启动扫描

`internal/fs/recovery.go`（或 `NewFS` 末尾）：遍历 staging 目录下所有 `*.staging` 文件，检查其 fid 是否指向活跃节点（`fs.nodes` 中能找到）。如果找不到，则删除。

## Risks / Trade-offs

| 风险 | 缓解措施 |
|------|---------|
| **内存占用增加**：Write 数据在内存 buffer 中保留最多 250ms | 最大 Page 大小限制（1MB），超量立即 flush；大文件不受影响 |
| **Crash 丢数据**：250ms 窗口内的写入未落盘 | Fsync/Release 强制 flush，保障用户保存语义。Crash 场景与修改前概率一致（最多丢最后一次未 fsync 的编辑） |
| **WriteAt 偏移覆盖**：编辑器的 Write 可能是乱序的 | Buffer 设计支持 offset 覆盖。同一 offset 后写入覆盖前写入，最终 flush 语义等同于多次 WriteAt 到磁盘 |
| **API 批量删除失败**：一次删除多个文件，部分成功部分失败 | 回退到逐条删除。不阻塞用户操作 |
| **crash 清理误删**：启动时删除正在被上传读取的 staging | 启动时 upload worker 尚未运行，没有并发操作 staging 的路径。仅清理 `fid 不在 nodes 中` 的文件 |
