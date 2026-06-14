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
- **Edge case suite** (`suite_edge.go`): Additional scenarios not in basic suite — `testMkdirExisting` (重名创建), `testDeleteNonExistent` (删除不存在), `testReadNonExistent`, `testListNonExistentParent`, `testSpecialCharsInName` (中文/空格/点号), `testLargeFileUpload` (100B~5MB+1), `testBatchRemove`, `testConcurrentPutSamePath`, `testDeepNestedDelete`, `testRepeatedUpload`. Register via `runEdgeSuite(t, drv)`.
- **Test isolation**: Each test creates its own `TestRoot()` subdirectory. `uid()` (pid + atomic counter) ensures unique names. `t.Cleanup()` removes test artifacts. Drivers must handle NotFound errors gracefully for non-existent entries.
- **FUSE-level test**: `hack/test-fs-ops.sh <mount_point>` — bash script testing POSIX ops (create, read, write, move, delete, permissions, symlinks) on a live FUSE mount. Not driver-specific; validates FUSE integration layer.
- **Mock drivers**: Each driver can have a mock package under its directory (`drivers/<name>/mock/`) for unit testing dependents without real API credentials.
- **Staging pattern**: release-before-write — staging dir captures file content before FUSE write returns, then async upload picks it up.
- **Upload sync**: debounce + retry loop. Uploader watches staging, uploads with backoff, reports progress.
- **FileAPI per-mount**: Each mount has its own `FileAPI` instance (driver + cipher + root_path). Avoid global state.

### Patterns

- **Tombstone for delete coalescing**: `delete.go` tracks `fid→bool` in `deletedFids` map (sync.Map). When a parent directory is being deleted (batch detect), subsequent child Unlinks set tombstone markers rather than submitting individual DELETE intents. `Create/Mkdir` must clear ancestor `deletingPaths` markers (`clearAncestorDeletingPaths`) and remove stale tombstones to prevent false-positive delete absorption — otherwise newly created files under a previously-deleted dir get silently ignored.
- **Batch delete detection**: `deleteBatchThreshold=20` — when N+ individual file Unlinks under the same parent are detected in `incDeleteBatchCounter`, `detectBatchDelete` proactively promotes to a single directory-level DELETE. Prevents the 952× API call amplification seen when Finder `rm -rf`s a large directory (Finder sends per-file Unlink + final Rmdir).
- **Write-back coalescer**: `OperationCoalescer` (in `coalescer.go`) batches adjacent UPLOAD/DELETE/MKDIR intents within a flush window. The `flush()` must check `intent.Upload.Started` before generating each intent (`MarkUploadStarted`) to prevent duplicates. Intents use write-back delay — coalesced dir DELETEs absorb late-arriving child Unlinks via `absorbDeleteUnderDeletingDir`.
- **Background cleanup on mount**: Recovery (`recoverFromCrash`) and staging cleanup (`cleanupStaleStaging`) run in background goroutines after mount returns. Don't block FUSE mount startup on potentially slow cleanup operations.
- **Upload staged logging**: Upload lifecycle logged in segments (`/file/create` duration, PUT part duration, `/file/complete` duration) via `logUploadSegment`. Helps identify which phase is the bottleneck — particularly useful for cloud drivers with varying API performance.

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

### Recent Refactoring Status (last verified: 2026-06-13)
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
- ✅ AppleDouble 处理 — `noappledouble` 配置项，`._*` 幽灵节点跳过远端列举，`lookupExtended` 性能优化
- ✅ Operation coalescer — 合并相邻的 UPLOAD/DELETE/MKDIR intent，防止重复上传
- ✅ FUSE 操作耗时日志 — 所有 FUSE 操作添加 `logFuseResult`，便于性能分析
- ✅ macFUSE unmount drain — 50ms drain 窗口避免 "Interrupted system call"
- ✅ Background recovery/staging cleanup — mount 不阻塞于清理操作
- ✅ Batch delete detection — 20+ 同目录 Unlink 折叠为单个目录 DELETE，避免 952× API 放大
- ✅ Tombstone 模式 — deletedFids 跟踪 + Create/Mkdir 清除标记防误吸收
- ✅ Write-back 延迟合并 — coalescer 批量 flush 相邻 intent，`absorbDeleteUnderDeletingDir` 吸收后续子删除
- ✅ 上传分段耗时日志 — `logUploadSegment` 分段记录 create/upload/complete 耗时
- ✅ Edge case 集成测试 — `suite_edge.go` 覆盖并发上传、深嵌套删除、特殊字符、大文件等场景

### Debugging & Problem-Solving Patterns (macOS + FUSE)

以下是从 AppleDouble/`._DS_Store` 支持调试过程中总结的通用排查方法：

#### 定位 Bug 的思路
1. **先确定是哪一层的问题**：FUSE 操作层 (`internal/fusefs/tree.go`) vs. 驱动层 (`drivers/`) vs. 内核/macFUSE 层（`/var/log/system.log` 或 `console` app 搜索 `"mFUSE"`）。三分钟无响应的 bug 通常不是远端慢，而是 FUSE 操作内的同步调用阻塞了内核线程。
2. **macFUSE 没有 strace**，替代方案：
   - 在 FUSE 操作入口加 `logFuseResult` 日志，记录操作名 + 耗时（参考 `getattr.go`、`readdir.go` 等文件的做法）
   - `sysctl vfs.generic.nclisten` 或 `fs_usage -w -f filesys /path/to/mount` 可以看内核侧挂载点的 I/O 事件
   - 如果 qrypt 假死，`kill -USR1 <pid>` 打 goroutine 堆栈（qrypt 注册了 `SIGUSR1` 处理）
3. **Finder 操作的特性**：
   - Finder 批量操作时并发度极高，`lookupExtended` 会被反复调用
   - 一次 `rm -rf 478 文件目录` 会触发 ~500 次 lookup，每次如果走 `fetchFiles`（~2s/次），总阻塞时间可达 14+ 分钟
   - 解决方案：已有节点跳过远端 fetch，只更新时间戳；AppleDouble 幽灵节点在远端列举之前创建

#### 常见 macOS/FUSE 问题及排查
| 症状 | 排查方向 | 常见根因 |
|------|----------|----------|
| Finder 复制目录报错 -36 / -50 | 检查 `lookupExtended` 对 `._*` 文件的处理 | AppleDouble `._*` 文件在远端不存在，FUSE 返回 ENOENT |
| 批量操作超时/挂起 | 加耗时日志，看哪个 FUSE 操作耗时异常 | `lookupExtended` 对已有节点仍执行 `fetchFiles` |
| 文件写入后不出现 | 检查 coalescer → UPLOAD intent → sync 链路 | intent 被重复生成或 sync 跳过 |
| unmount 后内核日志 "Interrupted system call" | 检查 `mount_fuse.go` 的 unmount 流程 | macFUSE 需要 drain 窗口（~50ms） |
| 文件拖拽到挂载点失败 | 检查 `mkdir` 是否走本地 + 远端双路径 | `Mkdir` 的 `source` 标记为 `local` 但未同步到远端 |

#### 常用排查命令
```bash
# 实时跟踪 FUSE 操作
fs_usage -w -f filesys /path/to/mount 2>&1 | grep qrypt

# 查看 macFUSE 内核日志
log stream --predicate 'subsystem contains "mFUSE"' --info

# 打印 qrypt goroutine 堆栈（排查死锁）
kill -USR1 <qrypt-pid>

# 手动测试 FUSE 挂载点
hack/test-fs-ops.sh /path/to/mount
```

### Common Gotchas
- **Session key stability**: SessionKey is in-memory only (not persisted), so changing key generation logic is safe to deploy.
- **Root path keys differ by driver**: quark→`root_path`, yun139→`root_id`, localfs→`local_root`. `RootPathForMount` now reads `m.Params[info.RootKey]`.
- **Cipher setter**: No compile-time check for `SetCipher` — uses interface assertion `drv.(interface{ SetCipher(cipher.Cipher) })`. If renaming, update all call sites.
- **Mock drivers**: quark and yun139 have mocks in test packages; always check mock coverage when adding new driver methods.
- **FUSE + concurrent writes**: Staging layer handles concurrent write races; check `staging/store.go` for locking guarantees.
- **WebSocket state reporting**: Reconnection triggers state snapshot via dashboard RPC.
- **Coalescer duplicate intents**: `flush()` 必须检查 `intent.Upload.Started` 防止每次 flush 都生成相同 UPLOAD intent，分发后调用 `MarkUploadStarted`。
- **AppleDouble 性能陷阱**: `._*` 的 ghost 节点必须在 `fetchFiles`（远端目录列举）之前创建，否则每次 lookup 都阻塞在慢速远端调用上。
- **Tombstone 失效**: `Create/Mkdir` 必须调用 `clearAncestorDeletingPaths` 清除祖先删除标记，否则新建文件会被 `absorbDeleteUnderDeletingDir` 吞掉。参考 `mkdir.go` 和 `tree.go` 中 `clearAncestorDeletingPaths` 的调用位置。
- **并发 Create 重复文件**: `sync.go` 中的上传后检查 `node.Cancel` 状态：如果两个 Create 同一路径并发，后完成的写入需要跳过上传（`skipUploadIfReplaced`），否则旧数据覆盖新文件。
- **Readdir 加锁**: `readdir.go` 的 `Readdir` 需要对 `MetadataTTL` 检查加锁，防止 `refreshDirAsync` 并发刷新导致数据竞争。

### Testing Patterns
- **Unit tests**: `_test.go` alongside source file
- **Integration test framework**: See `### Integration Test Framework` (Static Knowledge) — driver-agnostic suite in `internal/fusefs/integration/`
- **Mock drivers**: `drivers/quark/mock/` and `drivers/yun139/mock/` — use for testing without real API credentials
- **Config tests**: `internal/config/config_test.go` — test multi-path config loading, validation. Note: imports all drivers as side-effects to make `drivers.GetMeta` work.
- **FUSE-level test**: `hack/test-fs-ops.sh <mount_point>` — bash-based FUSE mount point POSIX op integrity validation
- **Race detection**: Mandatory `-race` flag on all test runs

### 测试实操经验

#### 分层测试策略

| 层 | 命令 | 何时用 | 反馈速度 |
|----|------|--------|----------|
| **单元测试** | `go test -count=1 ./internal/fusefs/` | 每次改动后 | ~10s |
| **集成(localfs)** | `LOCALFS_ROOT=/tmp/qrypt-int-test go test -count=1 -tags=integration -run 'TestConfiguredMounts$' ./internal/fusefs/integration/` | FUSE 层改动后 | ~1s |
| **集成(真实云)** | `YUN139_AUTH=xxx YUN139_ROOT_ID=xxx go test -count=1 -tags=integration ./internal/fusefs/integration/` | 云驱动改动后 | ~30s+ |
| **手动挂载测试** | `./qrypt mount 139yun` + Finder 操作 + 检查日志 | 端到端验证 | 分钟级 |

#### 集成测试添加流程

1. **写测试函数** `testXxx(t, drv)` 在 `suite_edge.go`（新场景）或 `suite_fileops.go`（基础 ops）
2. **注册** 到 `suite.go` 的 `runSuite` 中 `t.Run("Xxx", func() { testXxx(t, drv) })`
3. **优先 localfs 验证**：`LOCALFS_ROOT=/tmp/qrypt-int-test go test -tags=integration -v -run 'Xxx'`
4. **再云驱动验证**（需要凭据）：`YUN139_AUTH=... go test -tags=integration -v -run 'Xxx'`

#### 关键测试模式

**并发写入测试** — 两个 goroutine 同时 `Put` 同路径：
```go
// 一个结果成功即可（localfs 覆盖，云驱动可能 auto_rename）
entries, _ := drv.List(ctx, root)
for _, e := range entries { if e.Name == name { matching++ } }
if matching == 0 { t.Fatal("expected at least one file") }
```

**深层嵌套删除测试** — 10 子目录 × 2 子子目录 × 1 文件，BatchRemove 后验证根为空：
```go
for i := 0; i < 10; i++ {
    sub, _ := w.Mkdir(ctx, root, subName)
    leaf, _ := w.Mkdir(ctx, sub.ID, leafName)
    up.Put(ctx, leaf.ID, "f.txt", size, body)
}
br.BatchRemove(ctx, allEntries)
remaining, _ := drv.List(ctx, root)
if len(remaining) != 0 { t.Errorf("expected empty, got %d", len(remaining)) }
```

**重复上传测试** — 同位文件名 3 次 Put，验证只有一个文件：
```go
up.Put(ctx, root, name, size1, body1)
up.Put(ctx, root, name, size2, body2)
up.Put(ctx, root, name, size3, body3)
// List 后断言只有 1 个匹配项
```

**批量计数器单元测试** — `incDeleteBatchCounter` 19 次不触发，20 次触发：
```go
fs := newTestFS(t)
for i := 0; i < 19; i++ { fs.incDeleteBatchCounter("parent") }
// counter == 19, detectBatchDelete 未被调用
fs.incDeleteBatchCounter("parent")  // counter == 20, 触发
```

#### 手动挂载测试 CheckList

修改 FUSE 层逻辑后，必须在真实 mount 上验证：

1. `./qrypt mount 139yun` — 确认挂载成功
2. `ls ~/Qrypt139/` — 确认目录可见
3. 触发场景（创建/修改/删除目录）
4. `grep 'syncFile\|coalescer\|auto_rename\|Batch delete' ~/.qrypt/qrypt.log` — 检查关键日志
5. `./qrypt ls /` — 确认远端状态正确、无孤儿文件
6. `./qrypt rm -F "/xxx"` — 手动清理残留文件

#### 已知 Flaky Tests

| 测试 | 症状 | 原因 | 处理 |
|------|------|------|------|
| `TestRename` | `old path should be gone` | 时序依赖 | 重跑会过 |
| `TestE2E_LargeFileMultiChunk` | `read 0 bytes, want N` | staging 快照竞态 | 重跑会过 |
| `TestE2E_Truncate` | `got "\x00...", want "hello"` | 写入时序 | 重跑会过 |
| `TestDeleteNodePath_DirItself` | 偶发 panic | 树操作竞态 | 已知 pre-existing |

> 这些不是本次改动引入的。重跑 `go test -count=1 -run=<Name>` 通常能过。

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
