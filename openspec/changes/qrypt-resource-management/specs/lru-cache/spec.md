## MODIFIED Requirements

### Requirement: Bounded In-Memory Block Cache
The system MUST implement a capacity-bounded LRU cache for decrypted data blocks in memory (e.g., max 128MB or 2000 blocks) to prevent excessive memory consumption.

#### Scenario: Memory eviction
- **WHEN** the in-memory cache reaches its block limit and a new block is decrypted
- **THEN** the least recently used block is evicted from memory (but may remain on disk if configured)

### Requirement: LRU 自动清理机制
系统必须监控缓存目录的总大小，并在达到高水位线时自动清理最久未访问的分块。

#### Scenario: 缓存达到上限
- **WHEN** 缓存目录占用超过 20GB（配置上限）
- **THEN** 系统按访问时间升序删除旧分块，直到占用降低至 15GB
