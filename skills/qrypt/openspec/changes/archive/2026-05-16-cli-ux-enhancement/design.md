## Context

The `qrypt` CLI provides a robust suite of commands to interact with encrypted cloud drives. However, its current implementation for basic file operations (`ls`, `rm`, `mv`) lacks standard POSIX flags, and its transfer commands (`push`, `pull`) are strictly linear, lacking concurrency and recursive directory capabilities. This design document outlines the architectural changes required to bring POSIX parity to the base commands and to introduce a cloud-native, concurrent transfer engine.

## Goals / Non-Goals

**Goals:**
- Implement standard POSIX flags (`-R`, `-h`, `-t`, `-S`, `-f`, `-r`, `-i`, `-n`) for `ls`, `rm`, and `mv`.
- Introduce a new generic `Downloader` abstraction in `internal/sync` to mirror the existing `Uploader`.
- Build a generic task/worker-pool engine (concurrency model) for the `push` and `pull` commands.
- Implement remote folder creation capabilities within the drive abstraction to support recursive pushing.
- Introduce `sync` style flags like `--transfers N`, `--update`, and `--dry-run`.

**Non-Goals:**
- Implementing a full `sync` command (this will be a follow-up feature building on these primitives).
- Changing the underlying FUSE mount logic.
- Rewriting the cryptography implementation.

## Decisions

**1. Flag Parsing and Validation**
- We will leverage the existing `cobra` flag management to parse new parameters.
- For commands like `rm` and `mv`, explicit validation logic will be added to block operations on directories unless `-r/-R` (recursive) is provided.

**2. Downloader Abstraction**
- Currently, `pull` logic is embedded directly within `cmd/qrypt/pull.go`. We will extract this into `internal/sync/downloader.go`.
- The `Downloader` will expose an interface similar to the `Uploader` taking a `DownloadRequest` that encapsulates the source `fid`, destination path, and progress callbacks.

**3. Concurrent Transfer Engine (Worker Pool)**
- We will implement a `TransferPool` (or generic worker pool) that spins up `N` workers (`--transfers N`).
- A `Scanner` (or Walker) will run in a separate goroutine, recursively traversing local or remote directories, generating `TransferJob` structures, and pushing them to a work channel.
- Workers will pull `TransferJob`s from the channel and execute either the `Uploader` or `Downloader`.

**4. Remote Folder Creation**
- To support pushing a local directory tree, the Driver interface (`internal/drive/driver.go`) needs a mechanism to create remote directories.
- We will introduce a `CreateFolder(ctx context.Context, parentFid string, name string) (fid string, err error)` method to the driver layer.
- To prevent race conditions during concurrent pushes where multiple workers might attempt to create the same parent directory simultaneously, the traversal phase will pre-create the remote directory skeleton sequentially, or the folder creation logic will use a localized mutex/cache.

## Risks / Trade-offs

- **[Risk] High API Call Volume**: Recursive remote traversal (`pull`) without caching might trigger API rate limits.
  - **Mitigation**: Rely on the existing `cache` mechanisms or limit the max recursion depth by default unless overridden.
- **[Risk] Folder Creation Race Conditions**: Concurrent workers trying to push to a non-existent remote folder.
  - **Mitigation**: The scanner will handle directory creation before enqueuing file upload jobs, ensuring the `parentFid` is always available to workers.
