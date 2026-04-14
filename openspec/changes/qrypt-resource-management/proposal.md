## Why

As the volume of data handled by `qrypt` grows, current resource management strategies become insufficient. Specifically, the unbounded in-memory cache can lead to OOM errors, and the SQLite metadata database lacks automated maintenance, potentially impacting performance over time.

## What Changes

- Implement a capacity-bounded LRU for the in-memory block cache (`memCache`).
- Add automated SQLite maintenance (VACUUM, indexing optimization) and retention policies.
- Introduce disk space monitoring and alerts for `dirty_chunks` to prevent disk exhaustion.

## Capabilities

### New Capabilities
- `sqlite-maintenance`: Periodic optimization and cleanup of the local metadata database.

### Modified Capabilities
- `lru-cache`: Extend to support bounded in-memory caching in addition to disk-based LRU.

## Impact

- `skills/qrypt/internal/cache/db.go`: Add maintenance methods.
- `skills/qrypt/internal/vfs/fs.go`: Replace `sync.Map` with a bounded LRU for `memCache`.
- `skills/qrypt/internal/cache/manager.go`: Integrate memory LRU into the caching lifecycle.
