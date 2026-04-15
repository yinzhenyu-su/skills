## 1. Database & Cache Layer

- [x] 1.1 Update `pending_nodes` table schema with `base_server_mtime` and `base_server_size`
- [x] 1.2 Create `ops_log` table for journaling (id, op_type, source_path, target_path, payload, status, created_at)
- [x] 1.3 Update `SavePendingNode` and `GetPendingNodes` to handle new columns
- [x] 1.4 Implement `AddOpsLogEntry`, `UpdateOpsLogStatus`, and `GetPendingOpsLogs` in `CacheDB`

## 2. VFS Triple-State Model

- [x] 2.1 Update `node` struct in `internal/vfs/types.go` to include `baseServerMtime`, `baseServerSize`, and `lastMetadataCheck`
- [x] 2.2 Update `lookup` in `internal/vfs/path_state.go` to populate `baseServerMtime` and `baseServerSize` from `driver.File`
- [x] 2.3 Update `storeNode` and `replaceNodePath` to maintain triple-state fields

## 3. Metadata Refresh & TTL

- [x] 3.1 Implement TTL check in `lookup` (trigger refresh if expired)
- [x] 3.2 Implement `MergeRemoteChanges` in `internal/vfs/path_state.go` to sync local nodes with `driver.ListFiles` results
- [x] 3.3 Update `Readdir` in `internal/vfs/path_state.go` to trigger TTL-based refresh and call `MergeRemoteChanges`

## 4. Conflict Detection & Resolution

- [x] 4.1 Update `syncFile` in `internal/vfs/sync.go` to verify remote `updated_at` against `baseServerMtime` before upload
- [x] 4.2 Implement `resolveConflict` function to perform side-by-side rename (e.g., `file [Local Conflict].ext`)
- [x] 4.3 Trigger `resolveConflict` from `syncFile` and `MergeRemoteChanges` when a diverge is detected

## 5. Metadata Ops Journaling

- [x] 5.1 Refactor `Rename`, `Unlink`, `Rmdir`, and `Mkdir` to use `ops_log` (Journal-then-Call pattern)
- [x] 5.2 Implement `recoverPendingOps` to be called in `NewQryptFS` (scans `ops_log` and retries PENDING tasks)
- [x] 5.3 Ensure `ops_log` entries are cleaned up or marked DONE after successful API calls

## 6. Verification & Testing

- [x] 6.1 Unit test for `MergeRemoteChanges` with mock server lists
- [x] 6.2 Integration test for side-by-side conflict resolution (simulate server change during local edit)
- [x] 6.3 Recovery test for `Rename` (simulate crash after journal entry, verify retry on restart)
