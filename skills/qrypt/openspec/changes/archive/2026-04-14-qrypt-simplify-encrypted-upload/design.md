## Context

Qrypt currently couples write-back staging, dirty chunk persistence, in-memory chunk caching, rclone block encryption, multipart assembly, and Quark upload protocol calls inside `internal/vfs/fs.go`. The current flow writes modified data into many 64 KiB dirty chunks, then reconstructs encrypted multipart payloads during `syncFile`, and only reports hashes after multipart upload has already happened.

This design increases local I/O, spreads upload state across multiple layers, and diverges from the simpler AList Quark uploader shape. The target design keeps qrypt's rclone-compatible encryption but reshapes upload orchestration into the same high-level shape as AList for multipart transfer while preserving qrypt's proven Quark completion sequence: prepare local source, stream encrypted multipart data, verify the uploaded encrypted object hash, and fallback to commit/finish when required.

## Goals / Non-Goals

**Goals:**
- Move modified-file upload staging from chunk-oriented persistence to file-oriented local staging.
- Isolate upload orchestration into a dedicated manager instead of embedding it in `QryptFS.syncFile`.
- Introduce a streaming encryption reader that converts a plaintext staging file into rclone-compatible encrypted bytes during upload.
- Reorder qrypt internals around `UploadPre -> multipart upload -> UpdateHash -> Commit/Finish fallback`, so upload sequencing stays simple while remaining compatible with Quark's encrypted callback behavior.
- Use server-provided part size for multipart upload.
- Preserve dirty tracking, retryability, and flush/release-triggered synchronization semantics.

**Non-Goals:**
- Rewriting remote read caching or read-ahead logic for download paths.
- Changing qrypt's cryptographic format or compatibility with rclone crypt.
- Adding resumable multipart upload across process restarts in this change.
- Broadly redesigning unrelated cache eviction, metadata caching, or delete logic.

## Decisions

### 1. Use file-level staging instead of dirty chunk persistence

Modified files will be written into a single local staging file per path rather than a set of `.dirty.chunk` entries plus SQLite chunk metadata.

- **Why:** The upload pipeline becomes easier to reason about when upload input is a normal local file. It removes the need to rebuild file contents from many small persisted chunks before encryption and upload.
- **Alternative considered:** Keep chunk persistence and only refactor `syncFile`. This preserves current recovery behavior but leaves most of the complexity and local I/O amplification intact.

### 2. Split upload orchestration into a dedicated upload manager

`QryptFS` will keep FUSE-facing responsibilities (create, write, truncate, flush/release queueing), while a new upload manager owns upload preparation, hash computation, streaming encryption, and Quark multipart sequencing.

- **Why:** The current `syncFile` mixes state management and protocol steps. A dedicated manager makes the upload lifecycle testable and closer to AList's `Put` flow.
- **Alternative considered:** Keep orchestration in `fs.go` and extract only helpers. This reduces some function size but does not meaningfully simplify ownership boundaries.

### 3. Add a streaming encryption reader

The new encryption component will expose a sequential reader that emits:
1. rclone file header (`FileMagic + nonce`)
2. sequential encrypted blocks derived from the plaintext staging file

- **Why:** Multipart upload code should not need to understand rclone block boundaries. It should only consume encrypted bytes from a reader and split them into server-sized parts.
- **Alternative considered:** Continue building each multipart part by manually iterating plaintext blocks and buffering them inside `syncFile`. This keeps upload logic tightly coupled to encryption internals.

### 4. Verify encrypted object hashes after multipart upload

The upload manager will compute MD5/SHA1 over the encrypted byte stream while uploading parts, then call `UpdateHash` after multipart upload. If Quark responds with `finish=true`, qrypt will finalize locally without commit fallback; otherwise it will continue with commit and finish.

- **Why:** Real qrypt E2E behavior shows that encrypted uploads still require the existing post-upload hash verification order to avoid Quark callback failures.
- **Alternative considered:** Move hash reporting before multipart upload. This looked attractive for AList parity but reintroduced `203 CallbackFailed` for qrypt's encrypted upload path.

### 5. Use Quark-provided part size

Multipart upload will read encrypted bytes into buffers sized from `pre.Metadata.PartSize`, not from a locally fixed `8 MiB` strategy.

- **Why:** This aligns behavior with the server's upload contract and with AList's uploader, reducing local assumptions in protocol handling.
- **Alternative considered:** Keep the fixed local part size. This is simpler short-term but diverges from AList and may miss server-tuned sizing.

### 6. Keep file-level pending state, not chunk-level pending state

Pending upload persistence will record enough file-level metadata to retry a modified file upload, such as local staging path, path, parent fid, plaintext size, and upload lifecycle state. Chunk-level dirty state will no longer drive uploads.

- **Why:** Retryability remains possible without preserving per-block upload preparation state.
- **Alternative considered:** Preserve both file-level staging and chunk metadata during migration. This lowers migration risk but extends dual-write complexity and delays simplification.

## Risks / Trade-offs

- **[Risk]** Snapshotting staged files before upload adds one local file copy per sync attempt.  
  **Mitigation:** Accept the extra sequential copy as the cost of preserving a stable upload snapshot while removing many small random chunk writes and reads.

- **[Risk]** Moving away from dirty chunks reduces current chunk-level recovery semantics.  
  **Mitigation:** Preserve file-level pending metadata and local staging files so failed uploads remain retryable at the file level.

- **[Risk]** Refactoring `Write`, `Truncate`, and `syncFile` together can destabilize write-back behavior.  
  **Mitigation:** Migrate in stages: staging file writes first, upload manager second, chunk metadata cleanup last.

- **[Risk]** The server-provided part size may expose assumptions in current auth and upload helpers.  
  **Mitigation:** Reuse existing Quark protocol helpers and expand tests to cover part-size-driven upload behavior.

## Migration Plan

1. Introduce file-level staging storage and connect `Create`, `Write`, and `Truncate` to it while keeping existing upload queue entry points.
2. Add the upload manager and streaming encryption reader, then switch sync execution to consume staged files instead of dirty chunks.
3. Keep qrypt's proven completion sequence by verifying encrypted object hashes after multipart upload and before commit fallback.
4. Replace fixed multipart sizing with `pre.Metadata.PartSize`.
5. Shrink cache and pending metadata responsibilities by removing chunk-driven upload preparation from the write-back path.
6. Keep rollback simple during implementation by switching `syncFile` call sites back to the old flow until chunk-specific state is removed.

## Open Questions

- Should file-level pending metadata store the generated nonce before upload starts, or regenerate it on each retry attempt?
- Should local staging files be removed immediately after successful upload, or retained briefly for debugging/verification workflows?
- Is file-level retry sufficient for current qrypt needs, or do any existing workflows require true multipart resume semantics across restarts?
