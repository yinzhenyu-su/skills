# 修复 pending_nodes 残留数据

## 问题

pending_nodes 表中存在已上传完成但未清理的残留记录，原因：

1. **入口 1**（高频）：syncFile 成功后在 `n.isDirty=false` 与 `RemovePendingNode` 之间有约 20 行代码的崩溃窗口。Crash 后 DB 状态与内存状态不一致。
2. **入口 2**（低频）：重启后 `recoverDirtyFiles` 调 `lookupExtended` 失败（Quark 索引延迟或 `local_` 文件），直接 `continue` 不做任何处理，记录永久留在 DB。

## 方案

### Change 1 — 缩小崩溃窗口

把 `RemovePendingNode` + `staging.Remove` 移到 **锁内**（n.mu.Lock 块内），与 `n.isDirty=false` 同一事务：

```go
n.isDirty = false
// ...
localPath := n.localPath      // snapshot before clearing
// 锁内清理 DB + staging
if fs.cache != nil {
    _ = fs.cache.RemovePendingNode(path)
    if n.currentPath != "" && n.currentPath != path {
        _ = fs.cache.RemovePendingNode(n.currentPath)
    }
}
n.localPath = ""
if fs.staging != nil && localPath != "" {
    _ = fs.staging.Remove(localPath)
}
newFid := n.fid
n.mu.Unlock()
```

这样 isDirty=false 和 DB 清理是原子的——要么同时执行，要么都不执行。

### Change 2 — recoverDirtyFiles 兜底清理

`lookupExtended` 失败时，分三种情况：

| 条件 | 处理 |
|------|------|
| `fid` 不含 `local_` 前缀 且 `localPath=""` | 已上传但 DB 没来得及删 → **清理 DB 记录** |
| `fid` 含 `local_` 前缀 | 从未上传 → **从 DB 重建节点并重新入队** |
| 其他 | 日志警告，跳过 |
