## 1. Directory Metadata Caching

- [x] 1.1 Implement a `DirCache` struct in `internal/driver/quark.go` to store `[]File` with TTL.
- [x] 1.2 Modify `ListFiles` to check the cache before making API calls.
- [x] 1.3 Add negative caching for `FindChildByName` to avoid repeated failed lookups.

## 2. Parallel Pagination

- [x] 2.1 Refactor `ListFiles` to perform parallel HTTP requests for directories with multiple pages.
- [x] 2.2 Add a semaphore to `QuarkDriver` to limit the maximum number of concurrent API requests.

## 3. Intelligent Prefetching

- [x] 3.1 Extend the `node` struct in `internal/vfs/fs.go` to track sequential read metadata.
- [x] 3.2 Update the `Read` operation to detect sequential read patterns.
- [x] 3.3 Modify `prefetchBatch` to adjust the number of blocks to prefetch based on the read sequence length.

## 4. Verification

- [x] 4.1 Verify directory listing speed with and without cache.
- [x] 4.2 Verify parallel fetching for directories with 200+ files.
- [x] 4.3 Verify prefetching is triggered only for sequential reads.
