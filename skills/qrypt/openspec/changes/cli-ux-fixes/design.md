## Context

The previous change (`cli-ux-enhancement`) successfully laid the groundwork for concurrent transfers and standard POSIX flags. However, code review revealed that the `Downloader.Download` method was naively translated from the old `pull.go` logic, which issues a separate `drv.Read` call for every 4KB block. This is highly inefficient. Furthermore, the `--update` flag in the worker pool was stubbed out but not fully integrated for uploads, leading to unnecessary uploads.

## Goals / Non-Goals

**Goals:**
- Optimize `Downloader.Download` to stream the entire file body using a single `drv.Read` and wrap it in a decrypting reader.
- Fix the `--update` logic in `internal/sync/pool.go` to correctly skip existing files with matching sizes.
- Fix the trailing space typo in `cmd/qrypt/mv.go`.
- Add test suites for `Downloader` and `WorkerPool` using a mock driver.

**Non-Goals:**
- Implementing advanced checksum-based diffing for `--update` (size matching is sufficient for now).

## Decisions

**1. Downloader Streaming Optimization**
- Instead of looping and calling `drv.Read(..., offset, 4KB)`, we will call `drv.Read(..., headerSize, totalBodySize)` once to get an `io.ReadCloser` for the entire body.
- We will wrap this `io.ReadCloser` with `crypt.NewDecryptingReader()` (similar to how `Uploader` uses `NewEncryptingReader`).

**2. Update Logic for Uploads**
- The `ScanLocalForUpload` function currently lists remote directories to find parent FIDs and check if directories exist. We will extend this to also cache the names and sizes of remote files in that directory.
- When creating a `TransferJob` for an upload, the scanner will check if the target remote file already exists and if its decrypted size matches the local file size. If so, and `--update` is true, the file will be skipped entirely (not submitted to the pool).

## Risks / Trade-offs

- **[Trade-off] Size-only diffing**: `--update` relies strictly on file size comparisons because computing and comparing remote hashes through encryption is prohibitively expensive. This means files modified locally but keeping the exact same byte length might be skipped. This is an acceptable trade-off for this implementation phase.
