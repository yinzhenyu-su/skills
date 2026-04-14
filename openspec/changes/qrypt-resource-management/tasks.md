## 1. Bounded Memory Cache

- [ ] 1.1 Implement a `SimpleLRU` struct with `Get` and `Put` methods and a fixed capacity.
- [ ] 1.2 Update `QryptFS` to use `SimpleLRU` instead of `sync.Map` for `memCache`.
- [ ] 1.3 Add a configuration parameter or flag for the maximum memory cache size.

## 2. SQLite Maintenance

- [ ] 2.1 Implement a `Maintenance()` method in `CacheDB` that performs `VACUUM`.
- [ ] 2.2 Schedule periodic maintenance at startup and after large-scale deletions.
- [ ] 2.3 Implement a metadata expiration policy for non-dirty chunks.

## 3. Resource Awareness

- [ ] 3.1 Implement a disk space check before writing dirty chunks in `CacheManager.PutChunk`.
- [ ] 3.2 Add logging/alerts for disk space or memory cache eviction thresholds.

## 4. Final Validation

- [ ] 4.1 Verify memory usage remains stable under heavy read loads.
- [ ] 4.2 Verify the SQLite database size is reduced after maintenance.
- [ ] 4.3 Verify the system correctly handles low-disk-space scenarios for writes.
