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

### Driver Integration Experience (Lessons from Quark & Yun139)

对接第三方云盘驱动的实战经验总结：

- **参考 Alist (github.com/AlistGo/alist) 而不是 API 文档**：云盘厂商的官方 API 文档通常过时或不完整。Alist 的 `drivers/<name>/` 代码是活文档，包含完整的签名算法、请求头、API 端点、错误处理。139 的 `calSign`、`mcloud-sign`、`personalCloudHost` 全部从 Alist 逆向而来。
- **逆向步骤**：Addition（配置参数）→ API 端点路径 → 签名算法 → 请求头 → token 刷新机制 → 分片上传流程。对照 Alist 逐个文件看：`meta.go` → `util.go` → `driver.go` → `types.go`。
- **编译期接口断言**：`var _ drivers.Driver = (*MyDriver)(nil)` 确保签名匹配，新增接口就在这加一行。
- **认证头格式**：139 的 `Authorization` 需要 `"Basic "` 前缀，Quark 用 cookie，不要假设格式。
- **API 域名可能动态分配**：139 的文件操作走 `personalCloudHost`（启动时从路由策略 API 获取），上传初始化走主站 `yun.139.com`。不要硬编码 API 地址。
- **上传三阶段**：`/file/create`（创建任务）→ HTTP PUT 到预签名 URL（上传分片）→ `/file/complete`（提交完成）。少任何一步文件都不会出现。
- **分片上传头**：即使上传到预签名 URL，139 的 OSS 仍需要 `Origin` 和 `Referer` 头，否则返回 400。
- **token 刷新**：139 的 token 有效期有时间戳，需要定时刷新（Alist 用 12h cron）。qrypt 目前只在 Init 时刷新，长时间运行的 daemon 可能过期。
- **三阶段验证**：写文件后依次验证 (1) `syncFilePostUpload` 打印 fid 替换 → (2) 重启 mount 后文件在 `ls` 中可见 → (3) `cat` 能读回正确内容。

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

### Reference Implementation Pattern

当实现第三方云盘驱动时，**Alist (https://github.com/AlistGo/alist)** 是最佳的参考实现来源：

- **定位驱动代码**: `drivers/<name>/` 目录下，每个驱动一个包
- **重点关注**:
  - `driver.go` — 主逻辑（List、Link、Upload 等）
  - `meta.go` — Addition 结构体（配置参数定义）
  - `util.go` — 工具函数（签名、token 刷新、请求封装）
  - `types.go` — 请求/响应类型定义
- **逆向步骤**:
  1. 看懂 Alist 的 Addition → 对应 qrypt 的 `mounts.params`
  2. 提取 API 端点路径（`/file/list`, `/file/upload/init` 等）
  3. 复制签名算法（如 139 的 `calSign` + `mcloud-sign` 头）
  4. 注意区分"主站 API"和"个人云 API"（如 139 需要 `ensurePersonalCloudHost` 动态发现 API 地址）
  5. 处理 token 刷新机制（Alist 通常用 12h cron）
- **139yun 案例**: 从 Alist 的 `drivers/139/` 逆向出了 `calSign`、`personalRequest`、`ensurePersonalCloudHost` 等核心逻辑，是 qrypt 目前最完整的参考驱动

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
