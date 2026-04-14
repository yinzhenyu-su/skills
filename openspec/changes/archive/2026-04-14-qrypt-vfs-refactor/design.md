## Context

`qrypt` currently concentrates path lookup, remote metadata overlay, decrypted read caching, local write-back staging, and background upload retry logic inside `internal/vfs/fs.go`. The resulting coupling makes routine changes to CRUD handling risky: rename and delete flows have to coordinate with pending sync state, the read path still carries legacy chunk-cache assumptions, and upload orchestration is harder to validate independently from FUSE entrypoints.

The current code already converges on a better architecture: writes land in file-level staging files, uploads already consume staged plaintext through `upload.Manager` and `crypt.EncryptingReader`, and tests exercise several regression cases around rename, dirty-node protection, sync retries, and delete cleanup. This change packages those emerging boundaries into explicit modules without changing the on-disk or remote encrypted format.

## Goals / Non-Goals

**Goals:**
- Split VFS responsibilities into focused modules so CRUD, read caching, write-back staging, and sync orchestration can evolve independently.
- Keep staged plaintext files as the canonical source for upload and retry.
- Preserve rclone-compatible filename and file-content encryption, Quark multipart ordering, and existing Finder/macFUSE compatibility behavior.
- Make rename, delete, and retry behavior consistent even when queued sync tasks outlive the original path string.

**Non-Goals:**
- Replacing the Quark driver API surface or changing Quark multipart semantics.
- Changing rclone crypt key derivation, filename encryption, block format, or size mapping.
- Redesigning macFUSE mount options or Finder metadata filtering.
- Introducing a new sync protocol, server-side metadata model, or cross-device cache coordination.

## Decisions

### 1. Keep `QryptFS` as the FUSE facade, move stateful workflows into dedicated VFS submodules
- **Decision**: Retain `QryptFS` as the cgofuse-facing entrypoint, but delegate behavior to focused internal modules such as path/index management, read path, write-back state, and sync orchestration.
- **Rationale**: This preserves the current command and mount integration while reducing the blast radius of changes inside `fs.go`.
- **Alternative considered**: Split each FUSE syscall into standalone files only. Rejected because it would spread the same shared state across many thin wrappers without clarifying ownership.

### 2. Make write-back staging the only upload source of truth
- **Decision**: Treat file-level staging files and their persisted pending metadata as the only canonical upload input. Read cache chunks remain a read optimization and recovery aid, not part of upload assembly.
- **Rationale**: The code already uploads from staged plaintext snapshots. Formalizing that boundary removes legacy chunk-oriented write-back coupling and simplifies retry behavior.
- **Alternative considered**: Reconstruct upload payloads from cached plaintext chunks. Rejected because it increases coordination complexity and duplicates logic already superseded by staging files.

### 3. Coordinate sync using stable file identity, not transient path strings
- **Decision**: Queue and deduplicate sync work around stable node identity / staging-backed file state, while still updating path metadata for logs, pending persistence, and user-visible lookup.
- **Rationale**: Rename and move can happen after a sync task is queued. Stable identity avoids duplicate uploads and path races while preserving the latest visible path.
- **Alternative considered**: Continue using path-first queue ownership and recover by scanning all nodes after rename. Rejected because it keeps rename correctness coupled to cache traversal.

### 4. Centralize rename and delete state convergence
- **Decision**: Move rename and delete convergence into shared helpers that update the node index, pending persistence, staging lifecycle, and cache cleanup together.
- **Rationale**: Delete and rename currently require cross-cutting cleanup. Centralizing this behavior reduces the risk that one call path updates nodes but forgets pending metadata or staging files.
- **Alternative considered**: Let each syscall method perform its own cleanup inline. Rejected because the current duplication is one of the reasons behavior is difficult to reason about.

### 5. Preserve compatibility by treating crypt and uploader components as stable boundaries
- **Decision**: Keep `crypt.RcloneCipher`, `crypt.EncryptingReader`, and `upload.Manager` as the compatibility boundary for encrypted upload behavior, and adapt VFS write-back code around them instead of reworking their format responsibilities.
- **Rationale**: rclone compatibility depends on stable filename encryption, file header emission, block encryption, and multipart ordering. Reusing the existing boundary lowers compatibility risk.
- **Alternative considered**: Move encryption logic back into VFS read/write helpers. Rejected because it would reintroduce the coupling this refactor is trying to remove.

## Risks / Trade-offs

- [Risk] Refactoring shared state can break subtle rename/retry behavior → Mitigation: preserve and extend regression coverage for pending sync, recursive path updates, and dirty-node protection before moving logic.
- [Risk] Module extraction could accidentally leave two sources of truth for file state → Mitigation: make staging + pending metadata the only write-back authority and keep read cache strictly advisory.
- [Risk] Delete cleanup may miss staging or queued sync state during the transition → Mitigation: centralize delete convergence and test both successful delete cleanup and failed delete preservation.
- [Risk] Upload compatibility regressions might not show up in unit tests alone → Mitigation: keep crypt/upload interfaces stable and rely on existing perf/E2E coverage as the final guardrail.
