## Why

Currently, `qrypt` performs fresh network requests for directory listings and file lookups on every FUSE operation, leading to high latency and poor responsiveness in large directories. Additionally, the prefetching logic is overly aggressive and lacks fine-grained control based on actual read patterns.

## What Changes

- Implement a Time-To-Live (TTL) based directory metadata cache to reduce redundant API calls.
- Enhance the directory listing logic to support parallel page fetching for large directories.
- Refine the prefetching strategy with sequential read detection to optimize bandwidth usage.

## Capabilities

### New Capabilities
- `qrypt-metadata-caching`: TTL-based caching for directory listings and file attributes to improve VFS responsiveness.

### Modified Capabilities
- `read-ahead-prefetcher`: Enhance with sequential read detection and adaptive window sizing.

## Impact

- `skills/qrypt/internal/vfs/fs.go`: Core logic for `Readdir`, `lookup`, and `Read` will be modified.
- `skills/qrypt/internal/driver/quark.go`: `ListFiles` will be updated for parallel fetching.
- `skills/qrypt/internal/cache/`: May need extension for metadata persistence if we decide to persist the cache.
