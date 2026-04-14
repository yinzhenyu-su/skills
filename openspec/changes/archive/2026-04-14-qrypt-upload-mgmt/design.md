## Context

`qrypt` currently provides a read-only FUSE interface to Quark Drive with rclone-compatible decryption. The existing `QuarkDriver` has some upload-related methods but they are incomplete (e.g., `UploadCommit` is empty) and not integrated into the VFS. The VFS (`fs.go`) only implements read operations. Adding write support requires coordinating local caching, rclone-compatible encryption, and Quark's multipart upload API.

## Goals / Non-Goals

**Goals:**
- **Full File Management**: Support `mkdir`, `rename`, `rmdir`, `unlink` (delete) via FUSE.
- **File Upload/Modification**: Support creating new files and modifying existing ones.
- **Rclone Compatibility**: Ensure newly uploaded files are encrypted in the standard rclone format (NaCl Secretbox + random 24-byte header nonce).
- **Write Performance**: Use a local cache to buffer writes and avoid synchronous network overhead during small random writes.

**Non-Goals:**
- **In-place Partial Updates**: We will not attempt to patch files on Quark Drive. Any modification will result in a full-file re-upload (consistent with rclone's own behavior for most cloud backends).
- **Conflict Resolution**: First-write-wins or last-write-wins; no sophisticated merging of concurrent cloud changes.
- **Streaming Upload for FUSE**: We will not upload blocks in real-time as FUSE writes them; we will use a "Write-back" approach.

## Decisions

### 1. Write-back Cache Strategy
- **Rationale**: FUSE often sends small (e.g., 4KB-128KB) and sometimes random writes. Rclone encryption requires fixed 64KB blocks and a sequential nonce. Streaming these directly to a multipart cloud API is extremely fragile.
- **Implementation**: 
    - FUSE `Write` calls are directed to the local cache.
    - Modified blocks are marked `is_dirty = 1` in the SQLite `chunks` table.
    - FUSE `Flush`/`Release` triggers the "Sync" process if the file is dirty.

### 2. Full-File Re-encryption and Upload
- **Rationale**: Since a modification usually invalidates the file's original nonce/header (for security and to match rclone's typical behavior), we will treat a modified file as a new upload.
- **Process**:
    1. Generate a new 24-byte random nonce for the file header.
    2. Read from local cache (for dirty blocks) or download/decrypt from cloud (for unmodified blocks) to reconstruct the full plaintext.
    3. Encrypt into 64KiB blocks using the new nonce.
    4. Stream to Quark's `UploadPart` API.
    5. On success, update the file's `fid` in the VFS node and clear `is_dirty`.

### 3. Quark API Extension
- **API Endpoints**: Use `/api/v2/file/create_dir`, `/api/v2/file/delete`, `/api/v2/file/rename`, and `/api/v2/file/move`.
- **Note**: Although the base URL in the current driver is `/1/clouddrive`, research suggests the `/api/v2/` prefix is used for these specific management operations. We will update `QuarkDriver` to handle both or verify the correct path.

### 4. VFS State Management
- **Node Tracking**: Update the `node` struct to include a `isDirty` flag and possibly a `localPath` if we use separate temporary files for new uploads.
- **Atomic Swap**: When an upload finishes, the local node metadata must be updated with the new `fid` and `size` provided by Quark.

## Risks / Trade-offs

- **[Risk] Data Loss on Crash** → **[Mitigation]** Use the `is_dirty` flag in the SQLite database to identify files that were never successfully uploaded. On next mount, we can prompt or automatically attempt to resume/retry these uploads.
- **[Risk] Disk Space Exhaustion** → **[Mitigation]** The LRU eviction policy already skips `is_dirty` chunks. We must ensure the user has enough space for at least one full copy of the largest file they are modifying.
- **[Risk] Upload Performance** → **[Mitigation]** Multipart upload allows parallelizing the upload of encrypted blocks, but the encryption itself is sequential due to the incrementing nonce.
- **[Trade-off] High Memory/Disk Usage during Sync** → By re-uploading the whole file, we trade bandwidth and local temporary storage for implementation simplicity and 100% rclone compatibility.
