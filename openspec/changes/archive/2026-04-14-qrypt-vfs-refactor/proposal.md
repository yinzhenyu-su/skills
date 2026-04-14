## Why

`skills/qrypt/internal/vfs/fs.go` currently mixes path indexing, remote metadata overlay, decrypted read caching, local staging, and background upload orchestration in a single module. That makes CRUD behavior hard to reason about and raises the risk that future fixes to sync or caching logic will break rclone-compatible encrypted mounts.

## What Changes

- Split the current VFS implementation into focused modules for path/index state, read path, write-back staging, and background sync orchestration.
- Simplify file encryption and upload flow so staged plaintext files remain the canonical source for sync, while keeping the existing Quark multipart and hash-verification sequence intact.
- Remove legacy coupling between write-back logic and chunk-oriented upload assumptions, keeping only the local cache responsibilities that are still needed for reads and recovery.
- Harden rename, delete, and retry flows so pending state, node paths, and sync scheduling remain consistent across module boundaries.
- Preserve compatibility with existing rclone crypt filename and file-content formats so mounts remain interoperable with rclone-encrypted Quark content.

## Capabilities

### New Capabilities
<!-- None. This change refactors existing qrypt behavior and formalizes invariants needed during the refactor. -->

### Modified Capabilities
- `qrypt-modify-writeback-stability`: tighten write-back and retry invariants so staged files, pending records, and sync scheduling remain stable after the VFS split.
- `qrypt-encrypted-stream-upload`: preserve the staged-file-to-encrypted-stream upload contract and rclone-compatible content format while simplifying upload orchestration boundaries.
- `qrypt-delete-consistency`: keep delete cleanup centralized so successful remote deletion still converges node, pending, staging, and cache state after the refactor.
- `recursive-node-path-updates`: ensure rename and move flows continue to update in-memory paths and pending sync state consistently when directory trees are relocated.

## Impact

- `skills/qrypt/internal/vfs/fs.go` will be decomposed into smaller VFS-focused modules.
- `skills/qrypt/internal/upload/manager.go` and `skills/qrypt/internal/crypt/` will remain the core upload/encryption path, but with clearer interfaces from VFS write-back code.
- `skills/qrypt/internal/cache/` and `skills/qrypt/internal/staging/` will need boundary cleanup so read cache, pending metadata, and staging lifecycle are no longer entangled through VFS internals.
- Existing unit, perf, and E2E tests under `skills/qrypt/internal/vfs/` will need selective updates to match the new module boundaries without weakening compatibility coverage.
