## 1. Flag Registration and Parsing

- [x] 1.1 Register `-R`, `-h`, `-t`, `-S`, and `--json` flags for `lsCmd` in `cmd/qrypt/main.go`.
- [x] 1.2 Register `-r`, `-R`, `-f`, `-i`, and `--dry-run` flags for `rmCmd` in `cmd/qrypt/main.go`.
- [x] 1.3 Register `-i` and `-n` flags for `mvCmd` in `cmd/qrypt/main.go`.
- [x] 1.4 Register `-u`, `--transfers`, and `--dry-run` flags for `pushCmd` and `pullCmd` in `cmd/qrypt/main.go`.

## 2. Base Command Refactoring (POSIX Parity)

- [x] 2.1 Refactor `runList` (`ls.go`) to support human-readable sizing (`-h`).
- [x] 2.2 Refactor `runList` (`ls.go`) to support recursive listing (`-R`).
- [x] 2.3 Refactor `runList` (`ls.go`) to support sorting (`-t`, `-S`) and JSON output.
- [x] 2.4 Refactor `runRm` (`rm.go`) to block directory deletion without `-r/-R` and support `-i` (interactive) and `-f` (force).
- [x] 2.5 Refactor `runMv` (`mv.go`) to check if destination exists and support `-i` (interactive) and `-n` (no-clobber).

## 3. Downloader Abstraction

- [x] 3.1 Create `internal/sync/downloader.go` defining the `Downloader` struct and interface.
- [x] 3.2 Extract decryption and downloading loop from `cmd/qrypt/pull.go` into `Downloader.Download`.

## 4. Concurrent Transfer Engine (Worker Pool)

- [x] 4.1 Create `internal/sync/pool.go` defining the `TransferJob` and generic worker pool.
- [x] 4.2 Implement local directory scanner (walker) for recursive `push` that yields `TransferJob`s.
- [x] 4.3 Implement remote directory scanner (walker) for recursive `pull` that yields `TransferJob`s.
- [x] 4.4 Wire up the worker pool in `runPush` to process concurrent uploads based on `--transfers`.
- [x] 4.5 Wire up the worker pool in `runPull` to process concurrent downloads based on `--transfers`.
- [x] 4.6 Implement `--dry-run` logic in both push and pull scanners/workers to bypass actual API calls.

## 5. Driver Enhancements

- [x] 5.1 Define `CreateFolder` in `internal/drive/driver.go` interface.
- [x] 5.2 Implement `CreateFolder` in the Quark driver (`internal/drive/quark`).
- [x] 5.3 Integrate `CreateFolder` into the push scanner to pre-create remote directories before worker execution.
