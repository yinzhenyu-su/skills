## Why

Several stability issues have been identified in the `qrypt` FUSE implementation: macOS users encounter `Operation not permitted` errors during writes; directory renames leave children with stale path mappings in the cache; and concurrent writes during an active background sync can lead to data inconsistency.

## What Changes

- Update FUSE mount parameters to resolve macOS permission issues (`defer_permissions`, etc.).
- Implement recursive path updates for directory renames to ensure all nested file caches remain valid.
- Introduce per-node read-write synchronization or snapshotting to prevent race conditions during background uploads.

## Capabilities

### New Capabilities
- `recursive-node-path-updates`: Automated invalidation or updates of child node paths in the `fs.nodes` cache during parent directory renames.

### Modified Capabilities
- `fuse-mount`: Update mount flags for improved compatibility with macOS and other platforms.
- `qrypt-modify-writeback-stability`: Introduce synchronization mechanisms to handle concurrent writes during sync.

## Impact

- `skills/qrypt/cmd/qrypt/main.go`: Update mount options.
- `skills/qrypt/internal/vfs/fs.go`: Refine `Rename`, `Write`, and `syncFile` logic.
- `skills/qrypt/internal/cache/`: Ensure atomic updates to persistent metadata.
