## Why

The `qrypt` component currently suffers from several synchronization and retry bugs that lead to file corruption (0 KB files on Quark Drive) and silent sync failures. 

Critical issues identified:
1. **Early Dirty Bit Clearing**: `isDirty` is set to `false` *before* the sync completes. If sync fails (e.g., due to EOF or network error), the file remains "not dirty" and subsequent retries are skipped, leaving the remote file in an incomplete or 0 KB state.
2. **Path-Based Locking**: The sync worker uses path-based locking (`fs.syncing`). If a file is renamed while a sync is in progress or pending, it can be synced twice under different paths, leading to race conditions where a 0-byte sync might overwrite a valid data sync.
3. **Broken Renamed Retries**: Retry tasks use the original path. If the file is renamed, the retry task fails to find the node and aborts silently.

## What Changes

- Refactor `syncFile` to ensure `isDirty` is only cleared upon *successful* sync, and properly restored/maintained upon failure.
- Update `uploadWorker` to use node-based locking (identifying nodes by FID or unique ID) instead of path-based locking to correctly handle renames during sync.
- Improve the retry mechanism to follow nodes through renames, ensuring that a pending sync for a renamed file still finds its target.
- Add regression tests to verify sync reliability during concurrent renames and network failures.

## Capabilities

### New Capabilities
- None

### Modified Capabilities
- `qrypt-modify-writeback-stability`: Refine the requirements for sync failure handling and locking mechanisms to explicitly address rename scenarios and proper dirty state management.

## Impact

- `skills/qrypt/internal/vfs/fs.go`: Core logic for `uploadWorker`, `syncFile`, and `Rename`.
- `skills/qrypt/internal/vfs/fs_test.go`: Added regression tests.
- Quark Drive state: Prevent 0 KB file corruption.
