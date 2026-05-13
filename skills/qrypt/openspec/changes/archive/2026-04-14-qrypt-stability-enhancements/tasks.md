## 1. macOS FUSE Compatibility

- [x] 1.1 Update `main.go` to include `-o defer_permissions`, `-o local`, and `-o volname=QuarkDrive` in mount options.
- [x] 1.2 Implement dynamic UID/GID retrieval in `fs.Getattr` to match the current process.

## 2. Directory Rename Consistency

- [x] 2.1 Refactor `fs.Rename` to recursively update all child node paths in the `nodes` cache.
- [x] 2.2 Add unit tests for recursive renaming in `fs_test.go` or equivalent.

## 3. Write-Sync Coordination

- [x] 3.1 Add a per-node synchronization primitive to prevent `Write` / `syncFile` race conditions.
- [x] 3.2 Ensure `syncFile` takes a snapshot of the file state before initiating the upload.
- [x] 3.3 Validate consistency by simulating writes during a long-running sync.

## 4. Final Validation

- [x] 4.1 Reproduce and verify the fix for macOS `Operation not permitted`.
- [x] 4.2 Verify nested path lookups after directory rename.
- [x] 4.3 Verify data integrity of uploads with concurrent writes.
