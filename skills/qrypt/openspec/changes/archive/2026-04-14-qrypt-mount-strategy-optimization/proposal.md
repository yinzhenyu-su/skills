## Why

Users reported a "failed to save file 501 to .Trashes" error when deleting files from the mounted Quark Drive on macOS. This is caused by the `-o local` mount option, which tricks macOS into treating the FUSE volume as a local disk, prompting it to manage a local Trash folder. This behavior is incompatible with the encrypted remote file system and causes unnecessary API overhead and user frustration.

## What Changes

- Remove the `-o local` option from the FUSE mount parameters in `qrypt`.
- Update the global `fuse-mount` specification to explicitly forbid `local` mode for cloud-backed volumes.
- Add `-o noappledouble` to further reduce noise from macOS system files.

## Capabilities

### Modified Capabilities
- `fuse-mount`: Redefine the standard mount options for macOS compatibility.

## Impact

- `skills/qrypt/cmd/qrypt/main.go`: Update the `options` slice in the `mount` command.
- `openspec/specs/fuse-mount/spec.md`: Synchronize the documentation.
