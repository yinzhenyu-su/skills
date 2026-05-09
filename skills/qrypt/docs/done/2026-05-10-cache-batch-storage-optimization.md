# 缓存分块存储优化

## 问题

- `BlockDataSize = 64KB`，每个解密块单独一个磁盘文件
- 预览视频目录时，一个 2GB 视频产生 ~32,000 个 `.dec.chunk` 文件
- SQLite chunks 表同样 ~32,000 行
- `EvictIfNeeded` 定义了但没有任何地方调用，缓存只增不减

## 方案

### Change A — 合并磁盘存储

**不变的部分：**
- `BlockDataSize` 仍为 64KB（加密层不改）
- DB chunks 表仍按 `(fid, chunk_index)` 索引（查得快）
- CacheManager 接口不变（GetChunk/PutChunk 签名不改）

**改的部分：**

每次写缓存时，把连续的 N 个块合并到同一个文件里，而不是每个块一个文件。

合并粒度：`CacheBatchBlocks = 16`（16 × 64KB = **1MB 一个文件**）

**PutChunk 修改：**
```
写入块 i 时：
  batchIdx = i / CacheBatchBlocks
  fileName = "{fid}_batch_{batchIdx}.dec"
  打开文件，seek 到 (i % CacheBatchBlocks) * BlockDataSize 偏移
  写入解密数据
  DB 记录：file_path = 同一文件路径，chunk_index 仍独立
  写入完成 → close
```

**GetChunk 修改：**
```
读取块 i 时：
  path = DB 查到的文件路径
  打开文件，seek 到偏移量，读取 BlockDataSize 字节
  返回数据
```

**关键点：** `os.ReadFile` 改成 `os.Open` + `ReadAt`（只读需要的 64KB，不读整个 1MB 文件）

**DB 迁移：** 老的单文件缓存和新合并文件混用没问题——路径不同，自动兼容。

### Change C — 启用 EvictIfNeeded

PutChunk 每次写入后，采样检查是否需要驱逐：

```go
func (m *CacheManager) PutChunk(...) error {
    // ... 写入 ...
    m.evictCount.Add(1)
    // 每 100 次写入检查一次（避免每次写都查 DB）
    if m.evictCount.Load()%100 == 0 {
        go m.EvictIfNeeded(m.maxSize * 7 / 10)  // lowWatermark = 70%
    }
}
```

## 效果预估值

| 指标 | 改前 (2GB 视频) | 改后 (2GB 视频) |
|------|----------------|----------------|
| 磁盘文件数 | ~32,000 | ~2,000 |
| DB 行数 | ~32,000 | ~32,000（不变） |
| 磁盘上的元数据开销 | 32,000×inode | 2,000×inode |
| 逐出效率 | 从不逐出 | max_size 到上限时自动清理 |
