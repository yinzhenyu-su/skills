## Why

As the volume of data handled by `qrypt` grows, current resource management strategies become insufficient. Specifically, the unbounded in-memory cache can lead to OOM errors, and the SQLite metadata database lacks automated maintenance, potentially impacting performance over time.

## What Changes

- Replace unbounded `sync.Map` memCache with a capacity-bounded LRU using `golang-lru/v2`.
- Add automated SQLite maintenance (VACUUM, retention policy) and disk-space checks for staging writes.
- Introduce disk space monitoring for staging store to prevent disk exhaustion during dirty writes.

## Capabilities

### New Capabilities
- `sqlite-maintenance`: Periodic optimization and cleanup of the local metadata database.

### Modified Capabilities
- `lru-cache`: Extend to support bounded in-memory caching for decrypted block caching.

## Impact

- `skills/qrypt/internal/vfs/types.go`: Replace `sync.Map` with bounded `simplelru.LRU` for `memCache`.
- `skills/qrypt/internal/vfs/read.go`: Update `getDecryptedChunk` and `fetchBatch` to use new LRU API.
- `skills/qrypt/internal/cache/db.go`: Add `Maintenance()` method and retention policy.
- `skills/qrypt/internal/staging/store.go`: Add disk space check before writes.
