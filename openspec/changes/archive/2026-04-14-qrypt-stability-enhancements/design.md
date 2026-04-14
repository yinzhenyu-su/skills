## Context

Current stability issues in `qrypt` stem from incomplete integration with the operating system's FUSE expectations and race conditions in asynchronous background synchronization.

## Goals / Non-Goals

**Goals:**
- Eliminate `Operation not permitted` errors on macOS.
- Ensure 100% cache consistency after directory renames.
- Prevent data corruption or upload inconsistency during concurrent writes.

**Non-Goals:**
- Full lock-based POSIX compliance for multi-client concurrent access (single client focus).
- Redesigning the AList-based backend driver.

## Decisions

### 1. macOS Mount Parameters
- **Decision**: Update `host.Mount` to include `-o defer_permissions`, `-o local`, and `-o noappledouble`.
- **Rationale**: `defer_permissions` delegates permission checking to the VFS (which we handle in `Getattr`), avoiding kernel-level denials. `local` improves Finder responsiveness. `noappledouble` reduces `.DS_Store` noise.

### 2. Recursive Path Invalidation/Update in `Rename`
- **Decision**: When renaming a directory, iterate over `fs.nodes` (sync.Map) and update all keys that have the old path as a prefix.
- **Rationale**: Keeps the in-memory cache valid without forcing a full refresh of the entire hierarchy.

### 3. Per-Node Write-Sync Coordination
- **Decision**: Introduce a per-node `sync.RWMutex`. `Write` takes an `RLock`, while `syncFile` takes a `Lock` only during the short period when it snapshots the `dirty_chunks` and metadata.
- **Rationale**: Minimizes blocking of the user's write process while ensuring the upload task works on a consistent snapshot of data.

## Risks / Trade-offs

- [Risk] **High Memory Usage during recursive update** → [Mitigation] `fs.nodes` is already in memory; iteration is linear. For very large flat hierarchies, this could be slow, but it's consistent with current architecture.
- [Risk] **Deadlocks** → [Mitigation] Ensure a strict locking order (Parent -> Child, or Node -> Cache).
