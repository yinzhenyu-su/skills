## 1. Downloader Streaming Optimization

- [x] 1.1 Refactor `Downloader.Download` in `internal/sync/downloader.go` to use a single sequential read and `crypt.NewDecryptingReader`.

## 2. Incremental Update Logic

- [x] 2.1 Refactor `ScanLocalForUpload` in `internal/sync/pool.go` to cache remote file sizes.
- [x] 2.2 Implement skip logic in `ScanLocalForUpload` when `--update` is set and the decrypted remote size matches the local size.

## 3. Typo Fixes

- [x] 3.1 Fix the trailing space typo in `cmd/qrypt/mv.go` (change `dstArg, "/ "` to `dstArg, "/"`).

## 4. Test Coverage

- [x] 4.1 Create `internal/sync/downloader_test.go` to test the streaming downloader.
- [x] 4.2 Create `internal/sync/pool_test.go` to test the worker pool concurrency and skip logic.
