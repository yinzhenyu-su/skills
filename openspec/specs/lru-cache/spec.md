## ADDED Requirements

### Requirement: Bounded In-Memory Block Cache
The system MUST implement a capacity-bounded LRU cache for decrypted data blocks in memory (default max 512 entries ≈ ~32MB, enough for 4 prefetch batches of 128 blocks × 64KB each) to prevent excessive memory consumption.

#### Scenario: Memory eviction
- **WHEN** the in-memory cache reaches its entry limit and a new block is decrypted
- **THEN** the least recently used block is evicted from memory (but may remain on disk cache if configured)

### Requirement: 本地磁盘分块存储
系统必须将从云端下载或待上传的分块以文件形式存储在本地指定的缓存目录中。

#### Scenario: 写入缓存块
- **WHEN** 一个加密分块下载完成
- **THEN** 该分块被持久化到本地磁盘，并标记为可用

### Requirement: LRU 自动清理机制
系统必须监控缓存目录的总大小，并在达到高水位线时自动清理最久未访问的分块。

#### Scenario: 缓存达到上限
- **WHEN** 缓存目录占用超过 20GB（配置上限）
- **THEN** 系统按访问时间升序删除旧分块，直到占用降低至 15GB

### Requirement: 缓存元数据持久化
系统必须使用本地数据库（如 SQLite）存储分块的映射关系、访问时间和脏位标记。

#### Scenario: 重启后恢复缓存
- **WHEN** 程序重启并挂载
- **THEN** 系统加载 SQLite 数据库，能够识别并复用磁盘上已有的分块缓存
