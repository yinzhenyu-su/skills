## 1. Driver Modification

- [x] 1.1 Update the `File` struct in `internal/driver/types.go`: change the `Size` field type to `json.Number` and ensure the tag is `json:"size"`.
- [x] 1.2 Add `Int64Size()` helper to the `File` struct.

## 2. VFS Adaptation

- [x] 2.1 Update `internal/vfs/fs.go` to use `f.Int64Size()` instead of `f.Size` for all size-related logic.

## 3. Verification and Testing

- [x] 3.1 Add a unit test in `internal/driver/quark_test.go` to verify that both number and string formats for the `size` field are correctly unmarshaled.
- [x] 3.2 Verify that `rclone` decrypted size calculations work as expected with the non-zero file sizes from the driver.
- [x] 3.3 Run existing tests in `skills/qrypt` to ensure no regressions.
