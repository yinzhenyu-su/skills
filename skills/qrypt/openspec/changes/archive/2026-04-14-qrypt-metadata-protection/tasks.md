## 1. Node Struct Enhancement

- [x] 1.1 Add `syncQueued bool` to the `node` struct in `internal/vfs/fs.go`.

## 2. Metadata Protection

- [x] 2.1 Update `lookup` in `internal/vfs/fs.go` to avoid overwriting `isDirty` nodes.
- [x] 2.2 Update `Readdir` in `internal/vfs/fs.go` to avoid overwriting `isDirty` nodes.

## 3. Upload Deduplication

- [x] 3.1 Update `Flush` in `internal/vfs/fs.go` to check `syncQueued` before enqueuing.
- [x] 3.2 Update `uploadWorker` in `internal/vfs/fs.go` to reset `syncQueued` after processing.
- [x] 3.3 Ensure `Release` (which calls `Flush`) behaves correctly with deduplication.

## 4. Verification

- [x] 4.1 Run tests to ensure `isDirty` nodes are correctly protected.
- [x] 4.2 Manually verify that repeated `Flush` calls don't spam the "queued for upload" logs.
