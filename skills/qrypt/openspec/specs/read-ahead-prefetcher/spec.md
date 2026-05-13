## ADDED Requirements

### Requirement: 顺序读取检测与预取
系统必须在 FUSE Read 处理器中检测顺序读取行为，并在检测到后异步触发后续分块的下载。

#### Scenario: 顺序读取触发预取
- **WHEN** 连续 2 次以上的 Read 调用访问相邻分块（即 `offset == lastReadBlock+1 * BlockDataSize`）
- **THEN** 系统判定为顺序读取模式
- **AND** 启动预取协程，预取从 `endChunk+1` 开始的 `FetchBatchBlocks/4` 个分块（约 2MB）
- **AND** 预取以 batch 为单位（每组 `FetchBatchBlocks` ≈ 16 块），去重避免重复下载

#### Scenario: 随机读取
- **WHEN** 用户的读取请求不具有连续性（如随机 Seek）
- **THEN** 系统重置顺序计数器（`readSeqCount = 0`），不触发预取

### Requirement: 预取并发控制
系统必须使用信号量限制并发预取任务的数量（上限 30）。

#### Scenario: 预取任务限流
- **WHEN** 预取协程启动时
- **THEN** 尝试获取 `prefetchSem` 信号量（容量 30）
- **AND** 若信号量已满，直接返回不进行预取

### Requirement: 目录子节点预缓存
系统在 `Readdir` 中刷新当前目录列表后，应当对子目录发起后台预缓存。

#### Scenario: Readdir 触发子目录预取
- **WHEN** 用户 `ls` 一个包含子目录的目录
- **THEN** 对每个子目录检查其 `lastMetadataCheck` 是否超过 `MetadataTTL`（15s）
- **AND** 若过期且不在删除中，后台协程异步获取该子目录的文件列表并调用 `MergeRemoteChanges`
