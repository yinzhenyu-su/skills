## Why

The current `qrypt` CLI provides a solid foundation but lacks modern CLI conveniences and cloud-native network awareness, making complex folder operations tedious. We need to introduce POSIX-friendly flags (e.g., `-R`, `-h`, `--dry-run`) for basic commands and cloud-native semantics (e.g., concurrency, incremental sync, recursive folder uploads) for transfer commands, transitioning the user experience from single-file local tools to robust cloud-drive management.

## What Changes

- **POSIX Parity for Basic Commands**: 
  - `ls`: Add `-R` (recursive), `-h` (human-readable sizes), `-t` (sort by time), `-S` (sort by size), and `--json`.
  - `rm`: Enforce explicit deletion of directories using `-r/-R`, and add `-f` (force), `-i` (interactive), and `--dry-run`.
  - `mv`: Add `-i` (interactive prompt before overwrite) and `-n` (no-clobber).
- **Cloud-Native Transfer Enhancements**:
  - `push` & `pull`: Default to recursive folder traversal instead of rejecting directories.
  - Introduce `-u, --update` for incremental transfers.
  - Introduce `--transfers N` for concurrent worker-pool execution.
  - Introduce `--progress` and `--dry-run` to preview actions.
- **Underlying Abstractions**:
  - Formalize a `Downloader` in `internal/sync` to handle `pull` logic symmetrically to the `Uploader`.
  - Introduce a generalized task scanning/walking mechanism to feed a worker pool for concurrent transfers.
  - Add remote `CreateFolder` capability to support recursive folder `push`.

## Capabilities

### New Capabilities
- `cli-ux`: Defines the standard flags, UX behavior, and output formats for CLI operations, including JSON support and progress tracking.
- `concurrent-transfer`: Defines the worker-pool based synchronization engine for `pull` and `push`, including recursive directory traversal, concurrency (`--transfers`), and remote folder creation.

### Modified Capabilities

- 

## Impact

- `cmd/qrypt/*`: Major overhaul of flag parsing and execution flow.
- `internal/sync`: Introduction of new `Downloader`, generic task engine/worker pool.
- `internal/drive`: New requirements on the driver interface to support recursive `List` or remote folder creation (e.g., `Mkdir`/`CreateFolder`).
