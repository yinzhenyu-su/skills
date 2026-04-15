## Why

As the volume of data handled by `qrypt` grows, current resource management strategies become insufficient. Specifically, the unbounded in-memory cache can lead to OOM errors, and the SQLite metadata database lacks automated maintenance, potentially impacting performance over time.

## What Changes

- Replace unbounded `sync.Map` memCache with a capacity-bounded LRU using `golang-lru/v2`.
- Add automated SQLite maintenance (VACUUM, retention policy) and disk-space checks for staging writes.
- Introduce disk space monitoring for staging store to prevent disk exhaustion during dirty writes.
- Introduce staging file lifecycle management to prevent orphaned staging files from accumulating.

## Capabilities

### New Capabilities
- `sqlite-maintenance`: Periodic optimization and cleanup of the local metadata database.
- `staging-lifecycle`: Tracking and cleanup of staging files to prevent orphaned file accumulation.

### Modified Capabilities
- `lru-cache`: Extend to support bounded in-memory caching for decrypted block caching.

## Impact

- `skills/qrypt/internal/vfs/types.go`: Replace `sync.Map` with bounded `simplelru.LRU` for `memCache`.
- `skills/qrypt/internal/vfs/read.go`: Update `getDecryptedChunk` and `fetchBatch` to use new LRU API.
- `skills/qrypt/internal/cache/db.go`: Add `Maintenance()` method, retention policy, and `staging_meta` table.
- `skills/qrypt/internal/cache/manager.go`: Add `CleanupStagingMetas()` and startup orphan cleanup.
- `skills/qrypt/internal/staging/store.go`: Add disk space check, metadata callbacks, and orphan cleanup.
- `skills/qrypt/internal/vfs/sync.go`: Update `cleanupPendingEntry` to clean staging metadata.
