## Context

The current `qrypt` FUSE implementation suffers from high latency due to the lack of directory metadata caching and serial API requests. Each `ls` or file access triggers one or more network requests to the Quark API, which is especially slow for directories with many files.

## Goals / Non-Goals

**Goals:**
- Reduce directory listing latency by at least 80% for cached directories.
- Accelerate the display of large directories (100+ files).
- Reduce unnecessary bandwidth usage from the prefetcher during random access.

**Non-Goals:**
- Persistent metadata caching (stays in memory for now).
- Full POSIX permission emulation.
- Changes to the underlying encryption format.

## Decisions

### 1. In-Memory TTL Cache for Directory Listings
- **Decision**: Wrap `ListFiles` results in a cache structure with a creation timestamp.
- **Rationale**: Simplest way to implement TTL without complex invalidation. A 60-second TTL is sufficient for most user interactions.
- **Alternative**: Using an LRU for metadata. *Rejected* for now as the number of directories typically fits in memory.

### 2. Parallel Pagination in `QuarkDriver.ListFiles`
- **Decision**: When `_fetch_total` indicates multiple pages, spawn goroutines to fetch subsequent pages concurrently.
- **Rationale**: Quark API returns 100 items per page. A directory with 500 items currently takes 5 serial requests. Parallelizing this reduces it to roughly the time of one request.

### 3. Sequential Read Detection for Prefetching
- **Decision**: Add `lastReadBlock` and `readSequenceCount` to the `node` struct.
- **Rationale**: Only trigger `prefetchBatch` when `readSequenceCount` exceeds a threshold (e.g., 2 consecutive blocks).
- **Adaptive Window**: Increase prefetch count if the sequence continues.

## Risks / Trade-offs

- [Risk] **Stale Metadata** → [Mitigation] Keep TTL short (60s) and allow manual refresh via `ls` (some FUSE clients handle this) or wait for TTL.
- [Risk] **API Rate Limiting** → [Mitigation] Parallel fetching will increase short-term QPS; monitor and add a semaphore to limit max concurrent API requests if needed.
