# Qrypt Hands-On

Quick-reference skill for working on the qrypt project. Assumes familiarity with Go, FUSE, and the project's AGENTS.md overview.

## Static Knowledge (stable, rarely changes)

### Project Setup & Build
- **Go 1.22+**: `go build -o qrypt ./cmd/qrypt`
- **Dependencies**: macFUSE (macOS) or libfuse (Linux)
- **Config**: TOML, search order: `./qrypt.toml` → `~/.config/qrypt/qrypt.toml` → `~/.qrypt.toml` → `/etc/qrypt/qrypt.toml`
- **Test**: `go test -race ./...` — always include `-race`
- **FUSE tests**: `skills/qrypt/hack/test-fs-ops.sh` for mount point integrity checks

### Architecture Patterns
- **Driver self-registration** (`database/sql` style): drivers call `drivers.Register("name", factory)` in `init()`, callers use `drivers.New(type, params)`. Never add switch-cases for driver creation.
- **MountParams**: `map[string]string`, not struct. Param access via `m.Params["key"]`, not `m.Params.Cookie`. TOML format unchanged (map → TOML table is seamless).
- **Core kernel separation**: `core/qrypt/` is platform-agnostic (no gomobile-incompatible deps). Backend drivers in `drivers/`. CLI/daemon layer in `cmd/` and `internal/`.

### Adding a New Driver
1. **Create package** under `drivers/<name>/` with your struct implementing `drivers.Driver` (`Init`, `Drop`, `List`, `Read`). Optionally implement `drivers.Writer` (`Mkdir`, `Move`, `Rename`, `Remove`), `drivers.Uploader` (`Put`), and/or `drivers.CipherSetter` (`SetCipher`).
2. **Self-register in `init()`**: Call `drivers.Register("name", DriverMeta{...})` with constructor, `ParamSpec` slice, `RootKey` (param for root path/ID), and `CredentialKey` (param for credential — used for session dedup).
3. **Interface assertions**: Add compile-time checks like `_ drivers.Driver = (*MyDriver)(nil)` and similarly for any implemented optional interfaces.
4. **Declare parameters** via `ParamSpec`: `{Key: "xxx", Required: true, Help: "description", Default: "val"}`. Supports types `"string"` (default), `"select"` (use `Options: "a,b,c"`), `"bool"`.
5. **ParamSpec options**: `ParamSpec.Type` supports `"string"`, `"select"`, `"bool"`. Use `ParamSpec.Options` as comma-separated values for select type.
6. **Query param metadata**: Use `drivers.GetMeta(name)` to look up a driver's `ParamSpec`, `RootKey`, `CredentialKey` dynamically — prevents hardcoding driver-specific keys in config layer.
7. **Register in factory**: Add `_ "github.com/yinzhenyu/skills/qrypt/drivers/<name>"` to `drivers/factory/factory.go`.
8. **Cipher support**: If the driver supports client-side encryption, implement `drivers.CipherSetter` (`SetCipher(c cipher.Cipher)`). The FUSE layer calls this after construction via interface assertion.
9. **Constructor pattern**: The `Ctor` function in `DriverMeta` receives `drivers.Params` (i.e. `map[string]string`). Extract and validate required params, return descriptive errors.
10. **Reference drivers** for each pattern:
    - `localfs` → simplest driver, no remote API, full CRUD
    - `quark` → remote API with cipher, dir cache, streaming upload
    - `yun139` → remote API without cipher, paginated list, streaming upload

### Integration Test Framework
- **Location**: `internal/fusefs/integration/` — driver-agnostic test suites.
- **Registration**: Add a factory function in `setup.go`'s `driverFactories` map. The factory extracts params from `map[string]string` and calls the driver's constructor.
- **Env override**: Add env-based config in `envBasedConfig()` (e.g. `QRYPT_COOKIE` for quark, `YUN139_AUTH` for yun139) so CI runs without a config file.
- **Test discovery**: Config mount with `test_enabled = true` is auto-discovered by `DiscoverTestMounts()`. Without it, `TestConfiguredMounts` skips.
- **Run command**: `go test -tags=integration -race ./internal/fusefs/integration/`
- **Suite structure**: `runSuite(t, drv)` runs grouped scenarios (FileCreateWriteRead, MkdirAndList, RenameFile, RenameCrossDir, DeleteFile, DeleteDir, OverwriteFile). Each scenario auto-skips if the driver lacks required optional interfaces via helpers like `skipIfNotUploader(t, drv)`.
- **Concurrency safety**: Tests use `uid()` (pid + atomic counter) for unique file/dir names. `TestRoot()` creates isolated subdirectories per test.
- **FUSE-level test**: `hack/test-fs-ops.sh <mount_point>` — bash script testing POSIX ops (create, read, write, move, delete, permissions, symlinks) on a live FUSE mount. Not driver-specific; validates FUSE integration layer.
- **Mock drivers**: Each driver can have a mock package under its directory (`drivers/<name>/mock/`) for unit testing dependents without real API credentials.
- **Staging pattern**: release-before-write — staging dir captures file content before FUSE write returns, then async upload picks it up.
- **Upload sync**: debounce + retry loop. Uploader watches staging, uploads with backoff, reports progress.
- **FileAPI per-mount**: Each mount has its own `FileAPI` instance (driver + cipher + root_path). Avoid global state.

### Code Conventions
- **Language**: Chinese CLI output, English code identifiers and comments
- **Encryption**: rclone-compatible EME-AES (filename) + NaCl Secretbox (content)
- **Driver interface**: `drivers/driver.go` defines Meta/Reader/Writer/Uploader
- **Exit codes**: 0=success, 1=error, 3=confirm-needed
- **Logging**: lumberjack log rotation, structured

### Key Locations
| What | Where |
|------|-------|
| CLI commands | `cmd/qrypt/` (cobra) |
| FUSE operations | `internal/fs/` (cgofuse) |
| Quark API client | `drivers/quark/` |
| Upload sync engine | `core/qrypt/uploader.go` |
| Staging store | `core/qrypt/staging.go` |
| Chunk cache | `core/qrypt/chunk_cache.go` |
| Driver factory | `drivers/factory/factory.go` |
| Driver registry | `drivers/registry.go` |
| Configuration | `internal/config/` |

## Dynamic Knowledge (check each session — may have changed)

### Recent Refactoring Status (last verified: 2026-06-10)
- ✅ Core v2 architecture — platform-agnostic kernel extracted
- ✅ Driver self-registration — switch-case eliminated
- ✅ MountParams `struct → map[string]string` — all driver-specific switch-cases removed from config layer
- ✅ WebSocket migration: `nhooyr.io/websocket` → `github.com/coder/websocket`
- ✅ Cipher package extraction: cipher interface + constants to `drivers/cipher/`
- ✅ Stream upload — no longer reads entire file into memory before upload
- ✅ Race fix — staging snapshot race + upload path detection resolved
- ✅ Integration test framework — `work_dir` + `test_enabled` config options, env-based test config
- ✅ yun139 personal cloud API — dynamic host resolution for personal cloud accounts
- ✅ CI — GitHub Actions release workflow for binary builds

### Common Gotchas
- **Session key stability**: SessionKey is in-memory only (not persisted), so changing key generation logic is safe to deploy.
- **Root path keys differ by driver**: quark→`root_path`, yun139→`root_id`, localfs→`local_root`. `RootPathForMount` now reads `m.Params[info.RootKey]`.
- **Cipher setter**: No compile-time check for `SetCipher` — uses interface assertion `drv.(interface{ SetCipher(cipher.Cipher) })`. If renaming, update all call sites.
- **Mock drivers**: quark and yun139 have mocks in test packages; always check mock coverage when adding new driver methods.
- **FUSE + concurrent writes**: Staging layer handles concurrent write races; check `staging/store.go` for locking guarantees.
- **WebSocket state reporting**: Reconnection triggers state snapshot via dashboard RPC.

### Testing Patterns
- **Unit tests**: `_test.go` alongside source file
- **Integration test framework**: See `### Integration Test Framework` (Static Knowledge) — driver-agnostic suite in `internal/fusefs/integration/`
- **Mock drivers**: `drivers/quark/mock/` and `drivers/yun139/mock/` — use for testing without real API credentials
- **Config tests**: `internal/config/config_test.go` — test multi-path config loading, validation. Note: imports all drivers as side-effects to make `drivers.GetMeta` work.
- **FUSE-level test**: `hack/test-fs-ops.sh <mount_point>` — bash-based FUSE mount point POSIX op integrity validation
- **Race detection**: Mandatory `-race` flag on all test runs

### Architecture Docs
| Doc | Content |
|-----|---------|
| `docs/core-architecture-v2.md` | Final v2 architecture, component relationships |
| `docs/driver-registry-refactor.md` | Driver registry design, rationale, patterns |
| `docs/refactoring-guide.md` | Refactoring completion checklist, migration tracking |
| `docs/sync-lifecycle.md` | Upload sync lifecycle, state machine |
| `openspec/changes/archive/` | All completed OpenSpec changes with design docs |

### Branch Convention
- Work on feature/refactor branches off `main`
- Commit messages prefixed: `feat/fix/refactor/test/docs(qrypt):`
- Verify with `go build ./... && go test -race ./...` before push
