## Context

As `qrypt` handles larger datasets, it needs more robust resource lifecycle management to avoid memory exhaustion and maintain metadata performance.

## Goals / Non-Goals

**Goals:**
- Bound memory usage by the in-memory decrypted block cache.
- Maintain SQLite performance over long-term use.
- Prevent system crashes due to disk exhaustion from staging writes.

**Non-Goals:**
- Global multi-node cache synchronization.
- Complex hierarchical LRU across different storage tiers.

## Decisions

### 1. Bounded In-Memory LRU Cache
- **Decision**: Replace `sync.Map` for `memCache` with `hashicorp/golang-lru/v2/simplelru.LRU`.
- **Rationale**: The project already includes `golang-lru/v2` as a dependency. Using the battle-tested library avoids implementing a custom doubly-linked list + map LRU from scratch. The cache stores decrypted blocks (`fid_idx -> []byte`) to avoid re-decrypting frequently accessed data.
- **Configuration**: Max entries configurable (default 512 entries ≈ ~32MB, enough for 4 prefetch batches of 128 blocks × 64KB each).

### 2. SQLite Maintenance Routine
- **Decision**: Add a `Maintenance()` method to `CacheDB` that runs `VACUUM` and `PRAGMA incremental_vacuum`. Invoked explicitly by the application layer during low-traffic periods (not auto-triggered during writes to avoid `SQLITE_BUSY` conflicts with concurrent operations).
- **Rationale**: SQLite databases become fragmented over time; periodic maintenance preserves query speed. Explicit invocation avoids conflicts with concurrent writes that would cause `SQLITE_BUSY` errors during high-traffic periods.
- **Retention Policy**: Delete non-dirty chunks with `access_time` older than 30 days during maintenance.
- **Concurrency**: `PRAGMA journal_mode=WAL` enables concurrent reads during writes. `SavePendingNode` retries with exponential backoff (3 attempts: 0ms, 10ms, 20ms) on `SQLITE_BUSY`.

### 3. Disk Space Awareness for Staging Writes
- **Decision**: In `staging.Store.WriteAt()`, check available disk space before writing. Return error if critically low (below 100MB).
- **Rationale**: Prevents partial/corrupt staging files if the host disk is full. Staging files hold user data before upload, so running out of disk here causes data loss.

## Risks / Trade-offs

- [Risk] **Memory Eviction causing Thrashing** → [Mitigation] Ensure the memory cache is large enough for a typical working set (≥512 entries ≈ 32MB, covering 4 prefetch batches).
- [Risk] **Maintenance blocking writes** → [Mitigation] Maintenance is opt-in (explicit call) so application controls timing. WAL mode + busy_timeout=10s + SavePendingNode retry provides resilience for normal operations.
- [Risk] **Disk space check overhead on every write** → [Mitigation] Check is fast (single syscall), hard-blocks at critical threshold (100MB).

## Architecture

```
Read Path:
  FUSE → getDecryptedChunk → memCache(LRU) ──hit──→ return
                            │miss
                            ↓
                       fetchBatch (download+decrypt) → Store to memCache + disk cache

Write Path:
  FUSE → staging.Store.WriteAt → dirty.chunk → upload worker

Maintenance:
  Explicit call by app during low-traffic → Maintenance() → VACUUM + delete old non-dirty chunks
```
