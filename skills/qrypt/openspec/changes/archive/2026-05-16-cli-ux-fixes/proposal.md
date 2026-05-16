## Why

During the implementation of the `cli-ux-enhancement` change, several critical issues were identified:
1. `downloader.go` issues separate `Read` calls for every 4KB block, causing extreme inefficiency and triggering rate limits or slow performance.
2. The `--update` flag logic in `push.go` (and `internal/sync/pool.go`) is currently a no-op placeholder and does not actually filter uploads.
3. A minor typo exists in `mv.go` where `strings.HasSuffix(dstArg, "/ ")` is checked instead of `""`.
4. There is a lack of unit tests covering the newly added generic transfer pool logic and path resolution logic.

This change aims to patch these gaps to ensure the enhancements are production-ready.

## What Changes

- **Downloader Efficiency**: Modify `Downloader.Download` to stream the entire file body using a single sequential `Read` operation from the driver, passing the stream through the decrypting logic, rather than looping block-by-block `Read` requests.
- **Push Update Logic**: Implement the actual incremental check in `pool.go`'s `handleUpload` or `ScanLocalForUpload` so that files with matching size on the remote are skipped when `--update` is used.
- **Typo Fix**: Correct `strings.HasSuffix(dstArg, "/ ")` to `strings.HasSuffix(dstArg, "/")` in `cmd/qrypt/mv.go`.
- **Test Coverage**: Introduce `downloader_test.go` and `pool_test.go` (mocking the driver) to verify the new concurrent and downloading behaviors.

## Capabilities

### New Capabilities
- `test-coverage`: Adding missing test coverage for `internal/sync`.

### Modified Capabilities
- `concurrent-transfer`: Updating implementation details of `--update` and downloading efficiency. (Note: The core requirements in the spec remain the same, but we will document the exact matching criteria for `--update` here).

## Impact

- `internal/sync/downloader.go`: Complete rewrite of the reading loop.
- `internal/sync/pool.go`: Add remote file listing check for local uploads to support `--update`.
- `cmd/qrypt/mv.go`: One line fix.
- `internal/sync/*_test.go`: New test files.
