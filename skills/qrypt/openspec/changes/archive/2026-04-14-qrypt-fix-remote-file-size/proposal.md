## Why

Currently, all remote files listed via the Quark Drive driver in `qrypt` show a size of 0 bytes in the FUSE mount. This occurs because the Quark API may return the `size` field as either a JSON number or a quoted string (e.g., `"12345"`). The existing `int64` field with `json:"size"` (or previously misconfigured `json:"file_size"`) fails to capture the value correctly if it is formatted as a string, resulting in a default value of 0. This mismatch prevents correct metadata synchronization for existing remote files.

## What Changes

- **Robust `File` parsing**: Use `json.Number` for the `Size` field in the `File` struct to handle both numeric and string-quoted values from the Quark API.
- **VFS Adaptation**: Update the VFS layer to use a helper method for converting the parsed `json.Number` to `int64`.
- **Metadata Synchronization**: Ensure that remote file sizes are correctly parsed and reflected in the VFS layer and FUSE mount.

## Capabilities

### New Capabilities
- None

### Modified Capabilities
- `quark-driver`: Update the metadata parsing logic to correctly identify the file size field from the Quark API response.

## Impact

- `internal/driver/types.go`: The `File` struct definition will be modified.
- `internal/vfs/fs.go`: Files listed via `Readdir` and `lookup` will now correctly report their decrypted sizes instead of defaulting to 0.
- FUSE Mount: Remote files will show their correct sizes in the file manager and terminal.
