## Context

The `qrypt` component uses a background `uploadWorker` to synchronize files to Quark Drive. The current implementation uses path-based locking and prematurely clears the `isDirty` flag, leading to synchronization failures during renames and silent failures after network errors.

## Goals / Non-Goals

**Goals:**
- Implement node-based locking for synchronization.
- Fix `isDirty` state management to ensure retries are reliable.
- Ensure pending syncs follow files through renames.
- Prevent race conditions that cause 0 KB files.

**Non-Goals:**
- Changing the underlying Quark Drive API or Rclone cipher logic.
- Implementing a persistent upload queue (out of scope for this fix).

## Decisions

### 1. Node-Based Locking
Instead of `fs.syncing` using the file path as a key, it will use the `*node` pointer (or a unique node ID).
- **Rationale**: A file's path can change during synchronization, but the node object remains the same. Locking by node ensures that a file is only synced by one worker at a time, regardless of its current path.

### 2. Deferred `isDirty` Clearing
The `isDirty` flag will only be cleared *after* a successful synchronization.
- **Rationale**: Currently, `isDirty` is cleared before `syncFile` starts. If `syncFile` fails, the node is no longer marked as dirty, and subsequent retry attempts skip it. By deferring the clear, we ensure that a failed sync leaves the node eligible for retry.

### 3. Path-Agnostic Upload Queue
The `uploadChan` will continue to carry paths for logging and initial lookup, but the worker will validate the node's state and identity before proceeding.
- **Rationale**: If a file is renamed from `/A` to `/B`, and `/A` is already in the queue, the worker picking up `/A` will see that `/A` no longer exists or points to a different node. It should gracefully handle this by either skipping or re-queueing with the new path if the node is still dirty.

### 4. Retry Logic with Path Resolution
When a sync fails and is scheduled for retry, the retry task will use the `*node` to determine the latest path if possible, or the worker will skip if the node is no longer reachable.
- **Rationale**: Ensures that retries don't fail just because a file was moved.

## Risks / Trade-offs

- **[Risk]** → Deadlocks if node locks and filesystem locks are not handled carefully.
  - **Mitigation**: Ensure a consistent locking order (always lock node `mu` before performing operations, and use `syncing` map for cross-worker coordination).
- **[Risk]** → Memory leaks in `retryState` or `syncing` maps.
  - **Mitigation**: Ensure keys are removed in `defer` blocks or upon terminal failure/success.
