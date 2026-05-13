## Why

Currently, `qrypt` is a read-only tool that only supports listing and reading encrypted files from Quark Drive. To make it a fully functional cloud drive mount tool, it must support file creation, modification, and management (deletion, renaming, mkdir) while maintaining 100% compatibility with rclone's encryption format.

## What Changes

- **Quark Driver Enhancement**: Implement missing Quark API endpoints for folder creation, file/folder deletion, renaming, and moving.
- **Multipart Upload Completion**: Finalize the `UploadCommit` and related logic in `QuarkDriver` to support reliable file uploads.
- **FUSE Write Support**: Implement `Create`, `Write`, `Mkdir`, `Unlink`, `Rmdir`, and `Rename` operations in the VFS layer.
- **Cache-then-Upload Strategy**: Implement a write-back mechanism where modifications are first stored in the local cache as "dirty" blocks and then re-encrypted and uploaded as a whole upon file closure.
- **Rclone-Compatible Encryption for Writing**: Implement the full rclone encryption flow for new files, including random nonce generation and block-by-block NACL Secretbox encryption.

## Capabilities

### New Capabilities
- `qrypt-upload-sync`: Manages the lifecycle of a file upload, coordinating between dirty cache blocks and Quark's multipart upload API.
- `qrypt-mgmt-ops`: Handles directory and file management operations (mkdir, rename, delete) by proxying them to Quark APIs.

### Modified Capabilities
- `quark-driver`: Add requirements for write-related API calls (CreateDir, Delete, Rename, Move).
- `fuse-mount`: Add requirements for write-related FUSE operations and the write-back cache behavior.

## Impact

- **QuarkDriver**: Significant expansion of API surface.
- **VFS (fs.go)**: Complexity increase to handle stateful writes and cache synchronization.
- **Cache**: The `is_dirty` flag in the SQLite database will now be actively used to track and recover pending uploads.
- **User Experience**: Users will be able to treat the mounted drive as a regular disk for basic file operations.
