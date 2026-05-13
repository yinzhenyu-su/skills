## ADDED Requirements

### Requirement: 缓存自动维护（纯内存索引 + 磁盘 batch 文件）
系统缓存架构：加密分块存储在磁盘 batch 文件中（`<fid>_batch_<N>.dec.batch`），索引信息全在内存中（`CacheManager.chunkIndex map[string]*fileChunkCache`）。无 SQLite 数据库。

通过 `MaintenanceStart()` 后台维护协程定期执行缓存清理。

#### Scenario: 周期性维护
- **WHEN** `MaintenanceStart()` 启动后台维护协程
- **THEN** 每 10 分钟执行一次 `Maintenance()`:
  1. 调用 `EvictIfNeeded(maxSize * 7/10)` 检查缓存水位线（低水位 = maxSize 的 70%）
  2. 调用 `CleanupStagingMetas(24h)` 清理 24 小时前的 abandoned staging 元数据
  3. 调用 `PurgeOpsLog(72h)` 清理 72 小时前的已完成 ops log 条目
  4. 调用 `reportStaleUploadIDs()` 扫描长期未完成的 uploadID 并发出 wanring

#### Scenario: 写入触发 Evict
- **WHEN** `PutChunk` 完成一次缓存写入，`evictCount` 递增
- **AND** `evictCount % 100 == 0`
- **THEN** 后台协程执行 `EvictIfNeeded(maxSize * 7/10)`

#### Scenario: EvictIfNeeded 执行逻辑
- **WHEN** `EvictIfNeeded(lowWatermark)` 被调用
- **THEN** 遍历 `chunkIndex` 计算所有分块的总磁盘占用
- **AND** 若 `total > maxSize`，计算目标释放量 `targetEvict = total - lowWatermark`
- **AND** 筛选所有非脏（`IsDirty=false`）分块，按 `AccessAt` 升序排列
- **AND** 逐块删除磁盘 batch 文件并从内存 `chunkIndex` 中移除索引
- **AND** 当 `evicted >= targetEvict` 时停止

#### Scenario: Staging Meta 清理
- **WHEN** `CleanupStagingMetas(24h)` 执行
- **THEN** 从 `stagingMetas` 内存映射中移除 `UpdatedAt` 超过 24 小时且 Status 为 `abandoned` 的记录

### Requirement: 缓存统计
系统通过 `CacheManager` 维护缓存统计，包括总大小、缓存文件数、evict 计数等，用户可通过 `qrypt status` 命令查看。
