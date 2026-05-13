## 1. Regression Testing

- [x] 1.1 Add a regression test in `fs_test.go` that simulates a sync failure and verifies the node remains `isDirty`.
- [x] 1.2 Add a regression test in `fs_test.go` that simulates a rename during a pending sync and verifies the node is still correctly synchronized.
- [x] 1.3 Add a regression test that simulates concurrent sync requests for the same node under different paths.

## 2. Core Sync Logic Improvements

- [x] 2.1 Refactor `fs.syncing` to use `*node` as the key for the sync lock map.
- [x] 2.2 Modify `syncFile` to defer setting `isDirty = false` until the finalization of the upload (either hash update or commit/finish).
- [x] 2.3 Update `uploadWorker` to look up the node's current path if the queued path is no longer valid or points to a different node.
- [x] 2.4 Ensure `retryState` is correctly managed by node identity to follow files through renames.

## 3. Error Handling and Cleanup

- [x] 3.1 Improve error logging in `uploadWorker` to include the current path and node FID for better debugging.
- [x] 3.2 Ensure `cleanupLocalUploadState` is only called for terminal, non-retryable errors.
- [x] 3.3 Add `parentFid` to `node` and `cache` to ensure sync follows directory moves.
- [x] 3.4 Refactor `syncFile` to use `n.parentFid` instead of path-based lookup.

## 4. Validation

- [x] 4.1 Run `go test ./internal/vfs/...` to verify all sync fixes and regression tests.
- [x] 4.2 Verify that the `Successfully synced` log only appears after a confirmed successful upload.
