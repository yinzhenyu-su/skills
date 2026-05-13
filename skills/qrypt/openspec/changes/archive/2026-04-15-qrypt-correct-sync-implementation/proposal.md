## Why

Current `qrypt` synchronization follows a simple read-through cache and write-back staging model, which lacks robust remote-to-local propagation and active conflict detection. This leads to potential data loss if a file is modified both locally and on the server, or if the server state changes while a local upload is pending. The system also lacks atomicity for metadata operations like renames, which can result in inconsistent states after network failures.

## What Changes

- **Triple-State Tracking**: Track `Base Server State` (mtime/size), `Local State` (dirty/staging), and `Remote State` (server current) for every node.
- **Active Metadata Refresh**: Implement TTL-based background and on-demand metadata synchronization to detect remote changes.
- **Side-by-Side Conflict Resolution**: Automatically rename conflicting local changes (e.g., `file [Local Conflict].txt`) instead of overwriting or failing.
- **Atomic Metadata Operations**: Implement a Write-Ahead Log (WAL) for `Rename`, `Move`, and `Delete` operations to ensure eventual consistency after crashes.
- **Enhanced Cache Schema**: Update SQLite schema to store `base_server_mtime` and `base_server_size`.

## Capabilities

### New Capabilities
- `qrypt-conflict-resolution`: Implementation of side-by-side renaming and conflict detection logic.
- `qrypt-ops-journaling`: A transaction log for atomic metadata operations (Rename, Delete, Mkdir) with recovery on mount.

### Modified Capabilities
- `qrypt-metadata-caching`: Update to support triple-state tracking (Base/Local/Remote) and active TTL-based refreshing.

## Impact

- **Internal VFS**: Major updates to `node` struct, `lookup`, `Readdir`, and `syncFile` logic.
- **Cache Layer**: SQLite schema migration for `pending_nodes` and new `ops_log` table.
- **Uploader**: Pre-upload check against `baseServerMtime` to detect server-side changes.
- **Driver**: Potential need for more granular metadata fetching if supported by Quark API.
