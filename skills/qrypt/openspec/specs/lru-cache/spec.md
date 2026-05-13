## ADDED Requirements

### Requirement: 内存分块 LRU 缓存
系统使用 `hashicorp/golang-lru/v2` 实现内存中的解密分块缓存，最大 512 条目（约 32MB）。

#### Scenario: 内存缓存读写
- **WHEN** `getDecryptedChunk` 读取解密后的分块
- **THEN** 先查询内存 LRU 缓存（`memCache.Get(key)`），命中则直接返回
- **AND** 未命中则从磁盘 batch 文件加载并解密，然后存入 LRU
- **AND** 缓存满时自动淘汰最久未使用的条目

### Requirement: 内存 LRU 自动淘汰
`golang-lru/v2` 库在 `memCache` 达到容量上限（512 条目）时自动淘汰最久未使用的条目，无需手动调用 Purge。

### Requirement: 磁盘分块缓存 (Batch File)
系统使用磁盘 batch 文件持久化存储已下载的加密分块，每个 batch 文件包含连续 16 个分块（`CacheBatchBlocks`），FileSystem 层通过 `CacheManager.chunkIndex`（内存 `map[string]*fileChunkCache`）维护分块索引。
- 非脏分块文件命名格式: `<fid>_batch_<N>.dec.batch`
- 脏分块文件命名格式: `<fid>_batch_<N>.dirty.batch`

#### Scenario: 写入缓存块
- **WHEN** 一个加密分块下载完成（或本地修改标记为脏）
- **THEN** `PutChunk(fid, chunkIndex, data, isDirty)` 写入 batch 文件
- **AND** 在 `chunkIndex` 内存映射中记录 `ChunkInfo`（文件路径、偏移、大小、访问时间、脏位）
- **AND** 每写入 100 次自动触发 `EvictIfNeeded` 检查

#### Scenario: 读取缓存块
- **WHEN** 需要读取某个分块
- **THEN** `GetChunk(fid, chunkIndex)` 查询内存 `chunkIndex` 映射
- **AND** 若命中，从磁盘 batch 文件读取指定偏移和大小的数据
- **AND** 更新 `AccessAt` 时间戳

#### Scenario: 缓存达到上限
- **WHEN** `EvictIfNeeded` 检测到分块总大小超过 `maxSize`（配置上限）
- **THEN** 系统收集所有非脏分块，按 `AccessAt` 升序排列
- **AND** 逐块删除磁盘文件并清理 `chunkIndex` 映射，直到目标释放量（`total - lowWatermark`）达到
- **AND** 只有非脏（`IsDirty=false`）分块可以被淘汰

### Requirement: 维护循环
系统后台运行 `MaintenanceStart` 协程，每 10 分钟执行一次 `Maintenance()`：
- 调用 `EvictIfNeeded(maxSize * 7/10)` 检查缓存水位线
- 调用 `CleanupStagingMetas(24h)` 清理 24 小时前的 abandoned staging 元数据
- 调用 `PurgeOpsLog(72h)` 清理 72 小时前的已完成 ops log

### Requirement: VFS 节点淘汰循环
系统在后台运行节点淘汰协程（`lruEvictionLoop`），每 15 分钟遍历 `fs.nodes`，清理满足以下条件的叶子节点（保留根目录和 dirty 节点）：
- 节点数量超过 50000 上限时触发淘汰
- 保留最近 1000 个活跃节点
- 非根目录、非 dirty、非正在删除的节点可被淘汰
