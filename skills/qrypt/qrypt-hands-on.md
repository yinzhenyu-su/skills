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
- ✅ Integration test framework — `work_dir` + `test_enabled` config options
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
- **Integration tests**: `internal/fs/e2e_test.go` — requires FUSE
- **Mock drivers**: `drivers/quark/mock/` and `drivers/yun139/mock/` — use for testing without real API credentials
- **Config tests**: `internal/config/config_test.go` — test multi-path config loading, validation
- **FS ops test script**: `hack/test-fs-ops.sh` — bash-based FUSE mount point integrity validation
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
