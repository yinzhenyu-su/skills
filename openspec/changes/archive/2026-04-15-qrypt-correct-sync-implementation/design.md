## Context

The current `qrypt` implementation uses a simple write-back staging model. While efficient, it lacks robustness when files are modified on both the server and locally. It also lacks atomic metadata operations, meaning a network failure during a rename can leave the local VFS out of sync with the remote.

## Goals / Non-Goals

**Goals:**
- Implement a triple-state model (Base Server, Local, Remote current) for each node.
- Detect and resolve conflicts using a side-by-side renaming strategy.
- Ensure atomicity for metadata operations (Rename, Delete, Mkdir) via journaling.
- Maintain compatibility with existing encrypted file headers.

**Non-Goals:**
- Automatic merging of file contents (not possible with end-to-end encryption).
- Real-time server-side push notifications (limited by Quark Drive API).
- Full POSIX permission synchronization.

## Decisions

### 1. Triple-State Tracking in SQLite
We will extend the `pending_nodes` table to store `base_server_mtime` and `base_server_size`.
- **Rationale**: By knowing the exact version the local edit was based on, we can definitively detect if the server has changed since the local edit began.
- **Alternative**: Comparing local mtime with remote mtime (Unreliable due to clock skew and FUSE mtime behavior).

### 2. Side-by-Side Conflict Resolution
When `Remote.updated_at > node.baseServerMtime` and `node.isDirty`, the local file is renamed to `filename [Local Conflict].ext`.
- **Rationale**: This preserves user work without complicated merge logic, which is impossible for encrypted binary data. It follows the pattern used by Dropbox and OneDrive.
- **Alternative**: Overwrite remote (Data loss) or Abort upload (Leaves user stuck).

### 3. Ops-Journaling for Metadata Operations
A new `ops_log` table will record the intent of metadata operations.
- **Rationale**: FUSE metadata operations like `Rename` involve remote API calls. If these fail or the process crashes mid-operation, the local VFS state becomes inconsistent. A journal allows for retry and recovery on the next mount.
- **Alternative**: In-memory retry only (Lost on crash).

### 4. Adaptive TTL Refresh in `Lookup` and `Readdir`
Instead of a static cache, we use a 60s TTL. If expired, `Lookup` or `Readdir` triggers a background or synchronous refresh from the server.
- **Rationale**: Balances responsiveness with eventual consistency.

## Risks / Trade-offs

- **[Risk] Migration of Existing Database** → **Mitigation**: SQLite migration logic in `NewCacheDB` to add missing columns without losing current pending uploads.
- **[Risk] Performance Impact of Journaling** → **Mitigation**: Use SQLite's WAL mode and keep the `ops_log` small, cleaning up `DONE` entries.
- **[Risk] High API Usage for TTL Refresh** → **Mitigation**: Only refresh the specific directory/file being accessed, and respect the 60s window.
