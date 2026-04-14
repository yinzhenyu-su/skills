## Context

The current `qrypt` implementation uses `-o local` to improve Finder responsiveness, but it triggers macOS's local Trash management. This leads to failures when the system tries to create `.Trashes/501` directories on an encrypted, remote-backed volume.

## Goals / Non-Goals

**Goals:**
- Resolve the `.Trashes` creation error on file deletion.
- Maintain disk name and basic permission handling.
- Keep the implementation simple without complex path interception for system folders.

**Non-Goals:**
- Implementing a full Trash mechanism for cloud drives.
- Improving Finder performance via metadata indexing (which requires `local`).

## Decisions

### 1. Revert to Network Drive Mode
- **Decision**: Remove `-o local` from the mount options.
- **Rationale**: macOS treats non-local volumes as network drives. When deleting files on a network drive, Finder prompts "Delete Immediately" instead of moving to Trash, bypassing the problematic `.Trashes` logic entirely.

### 2. Standardized Mount Parameters
- **Decision**: Use the following combination for macOS:
  - `-o rw`: Enable read-write access.
  - `-o defer_permissions`: Delegate permission checks to VFS.
  - `-o volname=QuarkDrive`: Human-readable label.
  - `-o noappledouble`: Suppress creation of `._` files.

## Risks / Trade-offs

- [Trade-off] **Finder Responsiveness** → Without `local`, Finder might not cache metadata as aggressively, potentially leading to slightly slower folder browsing. This is an acceptable trade-off for functional correctness.
- [Risk] **Spotlight Indexing** → Network drives are not indexed by default, which reduces IO but might limit search functionality in Finder.
