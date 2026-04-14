## Why

Qrypt's current upload path mixes dirty-chunk persistence, SQLite bookkeeping, block-level encryption, multipart assembly, and Quark upload protocol handling in one flow. This makes uploads slower than necessary, hard to reason about, and harder to align with the proven AList Quark uploader behavior.

## What Changes

- Replace chunk-oriented upload staging with file-oriented local staging for modified files.
- Restructure upload synchronization around an AList-style multipart flow: upload preparation, sequential part upload, encrypted hash verification, commit fallback, and finish.
- Introduce a streaming encryption layer that transforms a plaintext staging file into an rclone-compatible encrypted byte stream during upload.
- Keep hash reporting in the post-upload verification phase so qrypt remains compatible with Quark's encrypted upload completion path.
- Switch multipart sizing to the server-provided part size instead of a locally fixed 8 MiB strategy.
- Reduce the upload role of cache metadata and pending chunk state to simpler file-level pending upload state.

## Capabilities

### New Capabilities
- `qrypt-encrypted-stream-upload`: Covers file-level staging, streaming encryption, and multipart upload orchestration that mirrors AList's Quark upload flow while preserving qrypt's rclone-compatible encryption.

### Modified Capabilities
- `qrypt-modify-writeback-stability`: Changes write-back behavior from chunk-driven upload preparation to file-level staging and upload session management while preserving dirty tracking, retryability, and flush/release-triggered synchronization.

## Impact

- Affected code: `skills/qrypt/internal/vfs/fs.go`, `skills/qrypt/internal/cache/*`, `skills/qrypt/internal/driver/quark.go`, and new upload/staging/encryption helper modules.
- Affected behavior: file modification, upload preparation, retry state, pending state persistence, and multipart upload sequencing.
- Affected systems: local disk staging, Quark upload APIs, and qrypt's write-back reliability path.
