## Context

As `qrypt` handles larger datasets, it needs more robust resource lifecycle management to avoid memory exhaustion and maintain metadata performance.

## Goals / Non-Goals

**Goals:**
- Bound memory usage by the block cache.
- Maintain SQLite performance over long-term use.
- Prevent system crashes due to disk exhaustion from pending uploads.

**Non-Goals:**
- Global multi-node cache synchronization.
- Complex hierarchical LRU across different storage tiers.

## Decisions

### 1. Capacity-Bounded Memory Cache
- **Decision**: Replace `sync.Map` for `memCache` with a structure combining a `sync.Mutex`, a Map, and a Doubly Linked List for LRU.
- **Rationale**: Standard LRU pattern. Limits memory usage to a predictable size (e.g., 128MB).
- **Alternative**: Use an external LRU library. *Rejected* to keep dependencies minimal for a small Go project.

### 2. SQLite Maintenance Routine
- **Decision**: Add a `Maintenance()` method to `CacheDB` that runs `VACUUM` and `ANALYZE` at startup or after a large delete operation.
- **Rationale**: SQLite databases can become fragmented over time; periodic maintenance preserves query speed.

### 3. Disk Space Awareness for Dirty Blocks
- **Decision**: In `CacheManager.PutChunk`, if `isDirty=true`, check available disk space before writing.
- **Rationale**: Prevents partial/corrupt writes if the host disk is full.

## Risks / Trade-offs

- [Risk] **Memory Eviction causing Thrashing** → [Mitigation] Ensure the memory cache is large enough for a typical working set (e.g., several 8MB prefetch batches).
- [Risk] **Maintenance blocking startup** → [Mitigation] Run maintenance tasks in a background goroutine or after the initial mount is successful.
