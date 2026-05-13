## Context

Current implementation of `qrypt` allows `Readdir` and `lookup` to overwrite existing cache nodes with remote data, even if the node is `isDirty`. This leads to misleading file sizes (0 bytes) during upload. Also, `Flush` and `Release` trigger redundant upload tasks.

## Goals / Non-Goals

**Goals:**
- Prevent overwriting `isDirty` nodes with remote metadata.
- Deduplicate upload tasks in `Flush`/`Release`.
- Reduce redundant logging for upload queuing.

**Non-Goals:**
- Changing the underlying upload protocol or driver logic.
- Implementing a persistent write-ahead log (WAL) for uploads (already handled by `pending_nodes` SQLite table).

## Decisions

### 1. Conditional Cache Update
- **Decision**: In `lookup` and `Readdir`, before calling `fs.nodes.Store`, check if a node already exists for the given path and if its `isDirty` flag is `true`. If `isDirty` is true, skip the store operation for that specific node.
- **Rationale**: This preserves the locally-written size and state during the upload process.

### 2. Deduplication using `isDirty` and `syncQueued`
- **Decision**: Introduce a `syncQueued` flag in the `node` struct.
- **Decision**: In `Flush`, only queue a task if `isDirty` is true AND `syncQueued` is false.
- **Decision**: `syncQueued` is set to true when enqueued, and cleared in `uploadWorker` after `syncFile` finishes (or fails).
- **Rationale**: This prevents multiple `syncTask` objects for the same node from filling up the `uploadChan`.

## Risks / Trade-offs

- **[Risk] Metadata Stale after Sync** → **Mitigation**: Once `syncFile` completes, it updates `n.fid` and `n.isDirty = false`. The next `Readdir` or `lookup` will then correctly update the node from the remote state if needed.
- **[Risk] Race Condition in Flag Update** → **Mitigation**: Use `node.mu` to protect `isDirty` and `syncQueued` updates.
