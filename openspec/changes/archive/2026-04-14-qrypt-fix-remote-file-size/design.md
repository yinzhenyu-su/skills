## Context

The `qrypt` driver uses the Quark Drive API to fetch file lists. The current `File` struct in `internal/driver/types.go` expects a field named `size` in the JSON response to populate the file size. However, empirical research and comparison with other implementations (like AList) show that the Quark `/file/sort` API returns the byte count in the `file_size` field. As a result, all remote files are currently parsed with a size of 0, which breaks `rclone` decryption logic in the VFS layer because it cannot subtract the required headers and overhead from a zero-length file.

## Goals / Non-Goals

**Goals:**
- Correct the mapping between the Quark API response and the internal `File` struct.
- Ensure the VFS layer receives accurate file sizes for remote files.
- Enable successful `rclone` decryption of remote files by providing the correct encrypted size.

**Non-Goals:**
- Modifying the upload logic (which already uses `size` correctly in the request body, but might need verification on the response parsing if any).
- Changing the local cache schema (which already supports `int64` sizes).

## Decisions

### 1. Robust JSON Parsing for `File.Size`
- **Decision**: Use `json.Number` for the `Size` field with the tag `json:"size"` in `internal/driver/types.go`.
- **Rationale**: The Quark API primarily uses `size` for byte count, but may return it as either a number or a string. `json.Number` provides a robust way to unmarshal both formats without error.
- **Alternatives**: Using `int64` with `json:"size"`. Rejected because it fails if the API returns a string-quoted number. Using `json:"file_size"`. Rejected after further research indicating `size` is more standard for the `/file/sort` endpoint.

### 2. VFS Metadata Update
- **Decision**: Update `internal/vfs/fs.go` to use the new `f.Int64Size()` helper.
- **Rationale**: Since `f.Size` is now a `json.Number`, explicit conversion is required for use in size calculations and FUSE stat structures.


## Risks / Trade-offs

- **[Risk] API Inconsistency** → **Mitigation**: Some Quark API endpoints might use `size` while others use `file_size`. However, for `clouddrive/file/sort` (the primary listing API), `file_size` is the standard. If `UploadPre` or other management APIs return `size`, we should ensure they are handled correctly. In `qrypt`, `File` is primarily used for listing results.
- **[Risk] Existing Local Cache** → **Mitigation**: The local SQLite cache stores sizes as integers. Correcting the remote size will cause the VFS to update its internal `node` state with the correct values upon the next directory listing or file lookup.
