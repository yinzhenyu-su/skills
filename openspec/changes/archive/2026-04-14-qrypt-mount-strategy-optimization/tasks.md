# Tasks: Optimize Qrypt Mount Strategy

## 1. Code Implementation
- [x] 1.1 Modify `skills/qrypt/cmd/qrypt/main.go` to remove `local` and add `noappledouble`.

## 2. Specification Sync
- [x] 2.1 Update `openspec/specs/fuse-mount/spec.md` to reflect the change in mandatory flags.

## 3. Verification
- [x] 3.1 Verify that deleting a file in Finder now prompts "Delete Immediately" and succeeds without error.
- [x] 3.2 Verify that the volume still shows up with the correct name "QuarkDrive".
