## 1. Extract VFS state boundaries

- [x] 1.1 Introduce dedicated VFS submodules for path/index state, read path, write-back state, and sync orchestration while keeping `QryptFS` as the cgofuse-facing facade.
- [x] 1.2 Move shared node lookup, dirty-node overlay, and recursive rename path-update logic out of `internal/vfs/fs.go` into reusable helpers with the same observable behavior.

## 2. Simplify write-back and sync flow

- [x] 2.1 Refactor create/write/truncate/flush handling so file-level staging and pending metadata are the only write-back source of truth.
- [x] 2.2 Update sync queueing and worker coordination to deduplicate by stable file identity, preserve latest path metadata across rename/retry, and keep retryable failure semantics.
- [x] 2.3 Centralize delete and cleanup convergence so node index, pending metadata, staging files, and cache state are updated together.

## 3. Preserve encrypted upload and read compatibility

- [x] 3.1 Keep `upload.Manager` and `crypt.EncryptingReader` as the staged-file upload boundary while removing any remaining chunk-reassembly assumptions from VFS write-back code.
- [x] 3.2 Keep decrypted read caching, batch fetch, and prefetch behavior working after module extraction without reintroducing write-back coupling.
- [x] 3.3 Verify rclone-compatible filename handling, file header/block format, and Quark multipart ordering remain unchanged after the refactor.

## 4. Update validation coverage

- [x] 4.1 Refactor existing VFS regression tests to target the extracted modules while preserving coverage for dirty-node protection, rename during pending sync, delete convergence, and sync deduplication.
- [x] 4.2 Update perf and E2E tests only as needed to match the new module boundaries and confirm compatibility-sensitive flows still pass.
