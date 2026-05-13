## 1. Bounded Memory Cache

- [x] 1.1 Add `simplelru.LRU[string, []byte]` field `memCache` in `QryptFS` struct, replacing `sync.Map`.
- [x] 1.2 Update `getDecryptedChunk` to use `memCache.Get`/`memCache.Contains`/`memCache.Add` instead of `sync.Map` Load/Store.
- [x] 1.3 Update `fetchBatch` to use `memCache.Add` after decrypting blocks.
- [x] 1.4 Update `prefetch` to use `memCache.Contains` instead of `sync.Map` Load.
- [x] 1.5 Add `MemCacheMaxEntries` config field (default 512).
- [x] 1.6 Update `NewQryptFS` to initialize LRU with capacity.

## 2. SQLite Maintenance

- [x] 2.1 Add `Maintenance()` method in `CacheDB` that runs `VACUUM` and `PRAGMA incremental_vacuum`.
- [x] 2.2 Add retention policy: delete non-dirty chunks where `access_time < datetime('now', '-30 days')`.
- [x] 2.3 Add `PRAGMA optimize` on startup for query planning.
- [x] 2.4 Add `PRAGMA journal_mode=WAL` to enable concurrent reads during writes (improves multi-goroutine performance).
- [x] 2.5 Add `SavePendingNode` retry with exponential backoff on `SQLITE_BUSY` (3 retries: 0, 10, 20ms).
- [ ] 2.6 Application layer calls `Maintenance()` explicitly during low-traffic periods (not auto-triggered during writes to avoid `SQLITE_BUSY` conflicts).

## 3. Disk Space Awareness for Staging

- [x] 3.1 Add `checkDiskSpace()` in `staging/store.go` — reject at 100MB free.
- [x] 3.2 Call `checkDiskSpace` in `WriteAt` before writing.
- [x] 3.3 Call `checkDiskSpace` in `Create` before creating new staging file.

## 4. Staging Lifecycle Management

- [x] 4.1 Add `staging_meta` table in `CacheDB` (fid, local_path, created_at, updated_at, size, status).
- [x] 4.2 Implement `SaveStagingMeta`, `UpdateStagingMeta`, `RemoveStagingMeta`, `GetStagingMeta` methods in `CacheDB`.
- [x] 4.3 Add `MetaStore` interface to `staging.Store` and `SetMetaStore` setter.
- [x] 4.4 Update `staging.Store.Create()` to call `SaveStagingMeta`.
- [x] 4.5 Update `staging.Store.WriteAt()` and `Truncate()` to call `UpdateStagingMeta`.
- [x] 4.6 Update `staging.Store.Remove()` to call `RemoveStagingMeta`.
- [x] 4.7 Add `ListStagingFiles()` and `CleanupOrphanedStagingFiles()` to `staging.Store`.
- [x] 4.8 Call `cleanupOrphanedStagingFiles()` in `NewCacheManager()` at startup.
- [x] 4.9 Add `CleanupStagingMetas()` to `CacheManager` to clean abandoned > 24h.
- [x] 4.10 Update `cleanupPendingEntry` in `sync.go` to also call `RemoveStagingMeta`.

## 5. Final Validation

- [x] 5.1 Verify memory usage remains bounded under heavy read loads (test with > 1000 sequential reads).
- [x] 5.2 Verify the SQLite database size is reduced after maintenance runs.
- [x] 5.3 Verify the system correctly handles low-disk-space scenarios for staging writes (log warning / reject).
- [x] 5.4 Verify concurrent stress test passes (10 files concurrently written and synced).
- [x] 5.5 Verify orphaned staging files are cleaned up at startup when no pending node exists.
