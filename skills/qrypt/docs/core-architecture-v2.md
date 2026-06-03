# Qrypt Core v2: 平台无关核心库设计

> **设计基线:** 不考虑与现有 `internal/daemon/` 实现的兼容性。`core/qrypt/` 是 qrypt 架构的 kernel，所有平台入口（FUSE desktop、CLI、未来移动端）都是它的对等消费者。
>
> **v1 链接:** `docs/core-architecture.md`（v1 将 `core/` 视为"未来给移动端用的库"，本版修正此叙事）

---

## Context

### 当前结构的问题

```
cmd/qrypt/         CLI (cobra)
internal/daemon/   WS RPC + Service (1278 行) + MountManager + 6 个 kernel 概念（god package）
internal/fs/       FUSE filesystem
internal/{drive,crypt,sync,cache,staging,config,log}/
```

- FUSE 桌面特性与平台无关业务逻辑**强耦合**在同一个 module
- 业务逻辑（会话管理、加密、上传、限流、事件）散落在 `daemon/`，无法跨平台复用
- v1 重构产出 `core/qrypt/` 但与 `daemon/` 平行存在，未达成"消除重复"目标
- `internal/daemon/` 是 god package（20 文件 / 3855 行 / 7 关注点）
- 命名问题：`internal/crypt` 与 stdlib `crypto/` 命名易混；`internal/log` 与 stdlib `log` 实际冲突
- `internal/sync` + `internal/staging` 同属"延迟上传"两个阶段，拆成两个包过度
- 详情见 [Code Directory Structure](#code-directory-structure)

### 重构目标

`core/qrypt/` 是 qrypt 的 kernel。FUSE desktop、CLI、iOS、Android 都是**对等消费者**。

## End State

**qrypt = `core/qrypt/`（平台无关 kernel）+ 平台 adapter 层**

- FUSE desktop: `internal/daemon/`（lifecycle, ~200 行）+ `internal/mount/` + `internal/rpc/` 协调 `internal/fusefs/`
- CLI: `cmd/qrypt/` 调用 `core/qrypt.FileAPI`（无业务逻辑）
- 移动端: `mobile/` 通过 gomobile 消费 `core/qrypt`（仅 MobileAPI 包装）
- 桌面 Platform Services 实现：`internal/platform/`（desktop adapter），`cmd/qrypt/` 和 `internal/daemon/` 共享

每个 adapter ≤ 500 行胶水代码，零业务逻辑重复。完整目录见 [Code Directory Structure](#code-directory-structure)。

## Goals

1. `core/qrypt/` 是纯 Go，不依赖 `internal/`、FUSE、平台运行时
2. 业务逻辑（上传、加密、缓存、会话、限流）只在 `core/` 中存在一份
3. `go test ./core/qrypt/` 在 macOS / Linux / Windows 三平台通过
4. FUSE desktop 是 `core/` 的第一个消费者，所有现有功能不退化
5. gomobile bind `mobile/` 一次生成 AAR / XCFramework

## Non-Goals

- 不重写加密、上传同步、缓存的具体算法（`internal/cipher`、`internal/upload` 仅迁移位置）
- 不实现 iOS/Android UI
- 不实现 Platform Services（`NetworkConfig`/`BackgroundTask`/`AppLifecycle`）
- 不改变 CLI 命令语义
- 不改变 FUSE 挂载行为
- 不引入 DI 框架、代码生成、build tag 切换
- 不拆分 `core/qrypt/` 为多包（包边界 = API surface 边界，单包保证无循环依赖）

---

## Architecture

### 层次图

```
┌────────────────────────────────────────────────────────┐
│                   平台入口层 (Adapters)                  │
│  cmd/qrypt/         CLI 入口                            │
│  internal/daemon/   FUSE 挂载 + WS RPC + MountManager  │
│  mobile/            gomobile 消费方（未来）             │
└──────────┬─────────────────────────────────────────────┘
           │ 全部依赖 core/qrypt
           ▼
┌────────────────────────────────────────────────────────┐
│            core/qrypt/ — 平台无关 kernel                │
│  FileAPI  SessionManager  Orchestrator                  │
│  EventBus  ProgressHub  CacheInvalidator                │
│  RateLimiter  MountLister (interface)                   │
│  Driver/Writer/Uploader/Cipher/PathResolver interfaces  │
│  Error  FileEntry  Options                              │
└──────────┬─────────────────────────────────────────────┘
           │ core/ 不依赖 internal/，通过接口消费
           ▼
┌────────────────────────────────────────────────────────┐
│          internal/ — 具体实现（满足 core/ 接口）        │
│  backend/{quark,yun139,localfs}  cipher/  upload/  ... │
└────────────────────────────────────────────────────────┘
```

### 依赖方向（硬约束）

```
core/qrypt/  ──depends_on──►  Go stdlib + 第三方包
   (kernel)                     (不依赖 internal/、fusefs/、daemon/)

cmd/qrypt/  ──depends_on──►  core/qrypt/  +  internal/{config,platform}
internal/daemon/  ──depends_on──►  core/qrypt/  +  internal/{mount,rpc,fusefs,backend,cipher,upload,index,platform,...}
internal/cipher/  ──implements──►  core/qrypt.Cipher
internal/backend/  ──implements──►  core/qrypt.Driver + Writer + Uploader + PathResolver
internal/mount/  ──implements──►  core/qrypt.MountLister
```

**CI 检查：** `grep -r "yinzhenyu/skills/qrypt/internal" core/qrypt/` 必须无输出。

### Layer 边界

| Layer | 位置 | 职责 | 限制 |
|-------|------|------|------|
| **L0 Kernel** | `core/qrypt/` | 业务逻辑 | 不得 import `internal/...`，不得有 FUSE/Unix socket/平台 API |
| **L1 Implementation** | `internal/{backend,cipher,upload,index,mount,rpc,fusefs,config,logging}/` | 具体后端 | 实现 `core/` 接口，不导出跨 kernel 边界的类型 |
| **L2 Adapter** | `cmd/qrypt/`、`internal/daemon/`、`internal/platform/`、`mobile/` | 嵌入平台运行时 | 可依赖 L0 + L1 |

---

## Code Directory Structure

### 目标目录

```
qrypt/                                    # module root
├── core/
│   └── qrypt/                            # kernel，唯一 platform-agnostic
│       ├── api.go                        # FileAPI
│       ├── session.go                    # SessionManager + DriverFactory
│       ├── events.go                     # EventBus
│       ├── progress.go                   # ProgressHub
│       ├── upload.go                     # Orchestrator
│       ├── rate_limit.go                 # RateLimiter
│       ├── cache.go                      # CacheInvalidator + MountLister
│       ├── driver.go                     # Driver/Writer/Uploader/PathResolver/Cipher interfaces
│       ├── platform.go                   # DirResolver/CredentialStore
│       ├── types.go                      # FileEntry/Entry/Options
│       ├── errors.go                     # Error/ErrorKind
│       └── *_test.go
│
├── mobile/                               # gomobile 入口（未来）
│   └── mobile.go                         # MobileAPI 包装
│
├── internal/
│   ├── backend/                          # 云盘后端（原 drive/）
│   │   ├── driver.go                     # backend.Driver（满足 core/qrypt.Driver）
│   │   ├── factory/                      # 后端工厂
│   │   ├── quark/                        # 夸克实现
│   │   ├── yun139/                       # 移动云盘实现
│   │   └── localfs/                      # 本地 FS 实现
│   │
│   ├── cipher/                           # 加密（原 crypt/，避免与 stdlib crypto/ 命名混淆）
│   │   ├── cipher.go                     # RcloneCipher（满足 core/qrypt.Cipher）
│   │   ├── eme_aes.go
│   │   └── secretbox.go
│   │
│   ├── upload/                           # 异步上传（合并原 sync/ + staging/）
│   │   ├── pipeline.go                   # debounce + retry
│   │   ├── staging.go                    # write-before-upload
│   │   └── journal.go                    # pending.jsonl（重启恢复）
│   │
│   ├── index/                            # 内存索引（原 cache/，更准确命名）
│   │   ├── index.go                      # path → fid 映射
│   │   └── batch.go                      # batch 存储
│   │
│   ├── fusefs/                           # FUSE 文件系统（原 fs/）
│   │   ├── fs.go
│   │   ├── fs_nofuse.go                  # build tag
│   │   └── e2e_test.go
│   │
│   ├── mount/                            # 从 daemon/ 拆出
│   │   ├── manager.go                    # MountManager（实现 core/qrypt.MountLister）
│   │   ├── fuse.go                       # FUSE 挂载生命周期
│   │   └── instance.go                   # MountInstance
│   │
│   ├── rpc/                              # 从 daemon/ 拆出
│   │   ├── server.go                     # WS server
│   │   ├── client.go                     # WS client
│   │   ├── dispatch.go                   # 命令路由
│   │   └── protocol/                     # 消息类型
│   │
│   ├── config/                           # TOML 配置加载
│   ├── logging/                          # 日志（原 log/，避免与 stdlib log 命名冲突）
│   │
│   └── platform/                         # 桌面 Platform Services 实现
│       ├── dir_resolver.go               # DesktopDirResolver
│       ├── credential_store.go           # TomlCredentialStore
│       └── adapter.go                    # newFileAPI / driverAdapter / cipherAdapter
│
├── cmd/
│   └── qrypt/
│       ├── main.go                       # cobra 入口
│       ├── ls.go  cat.go  mount.go
│       ├── push.go  pull.go  mv.go
│       ├── rm.go  mkdir.go  find.go
│       ├── status.go  config.go  init.go
│       └── e2e_test.go
│
├── docs/
├── openspec/
├── go.mod
└── go.sum
```

### 现状 → 目标映射

| 现状 | 动作 | 去向 | 关键依赖 |
|------|------|------|---------|
| `core/qrypt/` | 保持 | 不变 | — |
| `internal/crypt/` | rename | `internal/cipher/` | 工具一键替换 import |
| `internal/drive/` | rename | `internal/backend/` | 同上 |
| `internal/sync/` + `internal/staging/` | merge | `internal/upload/{pipeline,staging,journal}.go` | 跨包合并 |
| `internal/cache/` | rename | `internal/index/` | 工具一键替换 |
| `internal/fs/` | rename | `internal/fusefs/` | 同上 |
| `internal/log/` | rename | `internal/logging/` | 避免 stdlib `log` 冲突 |
| `internal/daemon/` (3855 行, 7 关注点) | split | `internal/daemon/` (lifecycle ~200 行) + `internal/mount/` + `internal/rpc/` + 协调 `internal/fusefs/` | 重点工作量 |
| `cmd/qrypt/core.go` + `platform.go` | move | `internal/platform/{adapter,dir_resolver,credential_store}.go` | 桌面 / daemon 共享 |
| `mobile/` | create | 新建（gomobile 入口，未来） | — |

### 依赖图（重构后）

```
                        cmd/qrypt/
                             │
                             ▼
                internal/daemon/  ─────► internal/platform/ ──► core/qrypt/
                             │                                     ▲
                             ▼                                     │
                internal/{mount, rpc, fusefs} ─────────────────────┤
                             │                                     │
                             ▼                                     │
                internal/{backend, cipher, upload, index} ─────────┘

mobile/  ──────────────────────────────────────────────────────► core/qrypt/
```

**硬约束**：`core/qrypt/` 不出现在 `internal/` 包 import 列表中作为"被依赖方"以外的引用。

### 重构成本

| 任务 | 难度 | 风险 |
|------|------|------|
| 批量 rename（cipher/logging/index/fusefs/backend） | 低 | 工具一键替换 |
| 合并 sync/ + staging/ → upload/ | 中 | 跨包合并，需重排 import |
| 拆 daemon/ 为 4 包 | **高** | 3855 行移动，需保留所有 e2e 通过 |
| 移动 adapter 到 internal/platform/ | 低 | 文件级移动 |
| 新建 mobile/ 占位 | 极低 | 一个空文件 |

**建议顺序**：低风险 rename 先做（半天），拆 daemon/ 放最后（与 [Phase 1-3](#phase-1-sessioneventprogressratelimiterorchestrator-迁移1-天) 同步做）。

---

## Key Decisions

### D1: `core/` 是 kernel，不是 extracted library

v1 把 `core/` 视为"未来给移动端用的库"——抽象形状被 CLI 需求塑形，`daemon/` 仍持有全部实现成为旁观者。

v2 重新定位：`core/` 是 qrypt 架构的**重新组织**。原 `daemon/` 中的平台无关逻辑**迁入** `core/`，原 `daemon/` 退化为 FUSE adapter。

**实施含义：**
- 不允许 `daemon/` 与 `core/` 持有同名的并行实现
- `daemon.Service` 与 `core.FileAPI` 功能重叠时，保留 `FileAPI`，删除 `daemon.Service` 对应方法
- `daemon/service.go` 中除 FUSE 节点树、mount 状态、WS RPC handler 外的逻辑**全部删除**

### D2: `core/` 不得 import `internal/`

v1 文档建议避免但未强制，`api.go` 实际触线（`import internal/crypt`）。

v2 加固：
- CI 加 forbidden import 检查
- `core/qrypt.Cipher` 是**活契约**
- `*cipher.RcloneCipher`（位于 `internal/cipher/`）通过 `internal/platform/adapter.go` 中的 `CipherAdapter` 包装后注入 `FileAPI`

```go
// internal/platform/adapter.go
type CipherAdapter struct{ Inner *cipher.RcloneCipher }
func (a CipherAdapter) EncryptSegment(s string) string { return a.Inner.EncryptSegment(s) }
// ... 其余方法
```

### D3: 接口集收敛为 5 个，全部使用

```
Driver        List/Read/Init/Drop          (读操作)
Writer        Mkdir/Move/Rename/Remove     (写元数据)
Uploader      Put                           (写内容)
PathResolver  ResolvePath                   (路径↔ID)
Cipher        Encrypt/Decrypt Segment/Size  (加解密)
```

`Driver`/`Writer`/`Uploader`/`PathResolver` 满足 ISP（只读挂载只需 `Driver`）。`Cipher` 必须实现且被 `FileAPI.cp` 持有。

### D4: 删除 v1 设计的 3 个 Platform Services

v1 设计了 `DirResolver`/`CredentialStore`/`NetworkConfig`/`BackgroundTask`/`AppLifecycle` 五个接口。v2 评估后砍掉三个：

| 接口 | 决策 | 理由 |
|------|------|------|
| `DirResolver` | **保留** | 真实需求（cache/data/config 目录） |
| `CredentialStore` | **保留** | 真实需求（密码迁移到 Keychain/Keystore） |
| `NetworkConfig` | **砍掉** | 假想需求（代理/证书），等真需要时再加 |
| `BackgroundTask` | **砍掉** | 假想需求（iOS BGTaskScheduler） |
| `AppLifecycle` | **砍掉** | 假想需求（iOS suspend/resume） |

**理由：** YAGNI。当前无 iOS/Android 代码，把假想需求写进 API 等于在没消费的接口上写文档。

### D5: `FileAPI` 是**唯一**文件操作入口

```
FileAPI.List/Stat/Find/Mkdir/Move/Remove/Read/Write/Push/Pull
```

- `daemon.Service` 中对应的 ListDir/Mkdir/Remove/Move/PushStart/PullStart/Find **删除**
- WS RPC handler `case "list_dir"` 内部改为调 `FileAPI.List(...)`
- FUSE 节点树更新（`updateFuseNodes` 等）保留在 `daemon/`（FUSE 特有，不属于文件操作）

### D6: `FileAPI` 构造函数签名

```go
type Options struct {
    Cipher        Cipher           // 必传
    Dirs          DirResolver      // 必传
    Creds         CredentialStore  // 必传
    DriverFactory DriverFactory    // 必传
    RateLimitBPS  int64            // 0 = unlimited
    NumWorkers    int              // 默认 3
    MountLister   MountLister      // nil = standalone (无 mount 失效)
}

func NewFileAPI(opts Options) (*FileAPI, error)
```

**关键变化（对比 v1）：**
- 不再直接接受 `Driver`，而是 `DriverFactory`（每次按 SessionKey 创建）
- `FileAPI` 内部拥有 `SessionManager`
- CLI 不直接传 `Driver`，通过 `DriverFactory` 间接创建
- 必传参数 nil 时 `NewFileAPI` 返回错误（D11 砍掉 Noop 默认）

### D7: `MountLister` 在 `core/` 定义

`internal/daemon/cache_invalidator.go` 持 `*MountManager`（位于 `internal/mount/manager.go`）是迁移到 `core/` 的最大障碍。

```go
// core/qrypt/cache.go
type MountLister interface {
    ForEachRunningMount(fn func(name string) bool)
}
```

`*mount.Manager`（在 `internal/mount/manager.go`）实现此接口（一个方法）。`CacheInvalidator` 持 `MountLister`，只关心 mount name。

### D8: 事件类型用具体值，不用 `interface{}`

v1 `Event.Data interface{}` 在 gomobile 边界不可导出。

v2：
```go
type Event struct {
    Type      EventType
    Mount     string
    Timestamp int64
    Progress  *ProgressEntry  // 始终是具体类型
}
```

### D9: gomobile 兼容边界

策略：**`core/` 内部可用 `io.Reader`/`chan`/`func`/`interface{}`，导出 method 签名尽量简单，但不强制 build tag**。

- `FileAPI.Read` 返回 `io.ReadCloser`（桌面 CLI `cat` 需要流式）
- `mobile/MobileAPI` 在**单独的 `mobile/` 包**（不在 `core/`）包装 `FileAPI`：
  - `io.ReadCloser` → `[]byte`
  - `[]FileEntry` → JSON string
  - `*ProgressEntry` → JSON string
- `core/` 不需要 build tag 切换行为，避免 `//go:build gomobile` 分散关注

### D10: 单元测试是验收标准

- `core/qrypt/` 全部导出符号必须有 `_test.go`
- 测试用 mock（`mockDriver`/`mockCipher`/`mockMountLister`），不接真实云盘
- CI 强制 `go test -race -coverprofile`
- 覆盖率目标：FileAPI 100%，其他组件 ≥80%

### D11: 删除所有 `Noop*` 默认实现

v1 定义 `NoopDirResolver`/`NoopCredentialStore`/`NoopBackgroundTask`/`NoopAppLifecycle`。问题：
- 把"未配置"错误从编译期推到运行期
- 移动端不能用 Noop，必须实现 Platform Services

v2 删除所有 `Noop*`：
- `NewFileAPI(opts)` 中 `Cipher`/`Dirs`/`Creds`/`DriverFactory` 为 nil 时**返回错误**
- 桌面 CLI 用 `cmd/qrypt/desktop*.go` 提供具体实现

### D12: Build 产物

| 产物 | 命令 | 内容 |
|------|------|------|
| `qrypt` CLI | `go build -o qrypt ./cmd/qrypt` | core + internal/{config,platform} + cmd |
| `qrypt` FUSE mount daemon | 同上（`qrypt mount` 子命令） | core + internal/{mount,rpc,fusefs,backend,cipher,upload,index,platform,logging,config} + daemon |
| `libqrypt.aar` | `gomobile bind -target android -o libqrypt.aar ./mobile/` | core + MobileAPI（未来） |
| `QryptCore.xcframework` | `gomobile bind -target ios -o QryptCore.xcframework ./mobile/` | 同上（未来） |

`mobile/` 是 gomobile 入口目录，**不在 v2 实施范围**。Phase 6 仅做 bind 命令跑通，不实现 MobileAPI。

---

## Interface Contracts

### `Driver` / `Writer` / `Uploader` / `PathResolver`

```go
package qrypt

type Entry struct {
    ID      string
    Name    string    // 加密名
    IsDir   bool
    Size    int64     // 加密大小
    ModTime time.Time
}

type Driver interface {
    Init(ctx context.Context) error
    Drop(ctx context.Context) error
    List(ctx context.Context, parentID string) ([]Entry, error)
    Read(ctx context.Context, entry Entry, offset, size int64) (io.ReadCloser, error)
}

type Writer interface {
    Mkdir(ctx context.Context, parentID, name string) (Entry, error)
    Move(ctx context.Context, entry Entry, dstParentID string) error
    Rename(ctx context.Context, entry Entry, newName string) error
    Remove(ctx context.Context, entry Entry) error
}

type Uploader interface {
    Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (Entry, error)
}

type PathResolver interface {
    ResolvePath(ctx context.Context, path string) (string, error)
}
```

### `Cipher`

```go
type Cipher interface {
    EncryptSegment(plain string) string
    DecryptSegment(cipher string) (string, error)
    EncryptedSize(plainSize int64) (int64, error)
    DecryptedSize(cipherSize int64) (int64, error)
    GenerateRandomNonce() ([24]byte, error)  // NaCl Secretbox nonce
}
```

`GenerateRandomNonce` 暴露给 `FileAPI.Push` 用于生成文件 nonce。

### `DriverFactory` 与 `SessionManager`

```go
type SessionConfig struct {
    Type   string
    Cookie string
    Auth   string
    RootID string
}

type SessionKey struct {
    Type    string
    CredKey string  // sha256(cookie|auth)[:16]
}

type DriverFactory interface {
    CreateDriver(ctx context.Context, cfg SessionConfig) (Driver, error)
}

type Session struct {
    Drv      Driver
    RefCount int32
}

type SessionManager interface {
    Acquire(ctx context.Context, key SessionKey, cfg SessionConfig) (*Session, error)
    Release(ctx context.Context, key SessionKey)
}
```

`FileAPI` 内部使用 `SessionManager` 缓存 Driver 实例。CLI 通过 `DriverFactory` 注入，无需手动管理 Session。

### `DirResolver` / `CredentialStore`

```go
type DirResolver interface {
    CacheDir() string
    DataDir() string
    ConfigDir() string
}

type CredentialStore interface {
    Get(key string) (string, error)
    Set(key, value string) error
    Delete(key string) error
}
```

### `MountLister` / `CacheInvalidatorHooks`

```go
type MountLister interface {
    ForEachRunningMount(fn func(name string) bool)
}

type CacheInvalidatorHooks interface {
    InvalidateDirCache(mountName, dirPath string) error
}
```

### `Error`

```go
type ErrorKind int

const (
    ErrNotFound      ErrorKind = iota
    ErrAlreadyExists
    ErrNotEmpty
    ErrPermission
    ErrNetwork
    ErrInternal
)

type Error struct {
    Kind    ErrorKind
    Message string
    Cause   error
}

func (e *Error) Error() string
func (e *Error) Unwrap() error
func (e *Error) Is(target error) bool
```

### `FileAPI`

```go
type FileEntry struct {
    ID        string
    Name      string    // 加密名
    DecName   string    // 解密名
    IsDir     bool
    Size      int64
    PlainSize int64
    ModTime   time.Time
}

type PushOptions struct {
    OnProgress func(p *ProgressEntry)
}

type PullOptions struct {
    OnProgress func(p *ProgressEntry)
}

type FileAPI struct { /* unexported fields */ }

func NewFileAPI(opts Options) (*FileAPI, error)

func (a *FileAPI) List(ctx context.Context, mount, path string) ([]FileEntry, error)
func (a *FileAPI) Stat(ctx context.Context, mount, path string) (*FileEntry, error)
func (a *FileAPI) Find(ctx context.Context, mount, path, pattern string, maxDepth, maxMatches int, caseSensitive bool) ([]FileEntry, error)
func (a *FileAPI) Mkdir(ctx context.Context, mount, path string) error
func (a *FileAPI) Move(ctx context.Context, mount, oldPath, newPath string, opts MoveOptions) error
func (a *FileAPI) Remove(ctx context.Context, mount, path string, recursive bool) error
func (a *FileAPI) Read(ctx context.Context, mount, path string) (io.ReadCloser, error)
func (a *FileAPI) Push(ctx context.Context, mount, localPath, remotePath string, opts PushOptions) error
func (a *FileAPI) Pull(ctx context.Context, mount, remotePath, localPath string, opts PullOptions) error
func (a *FileAPI) Sessions() SessionManager
func (a *FileAPI) Events() EventBus
func (a *FileAPI) Progress() ProgressHub
func (a *FileAPI) Shutdown()
```

`MoveOptions`：
```go
type MoveOptions struct {
    NoClobber bool
}
```

### `EventBus` / `ProgressHub`

```go
type EventType string

const (
    EventSyncProgress  EventType = "sync_progress"
    EventSyncCompleted EventType = "sync_completed"
    EventSyncFailed    EventType = "sync_failed"
)

type Event struct {
    Type      EventType
    Mount     string
    Timestamp int64
    Progress  *ProgressEntry
}

type EventBus interface {
    Subscribe(id string) <-chan *Event
    Unsubscribe(id string)
    Publish(evt *Event)
}

type ProgressEntry struct {
    TaskID    string
    Mount     string
    Direction string  // "push" | "pull"
    File      string
    Bytes     int64
    Total     int64
    State     string  // "started" | "progress" | "completed" | "failed"
    Error     string
    UpdatedAt time.Time
}

type ProgressHub interface {
    Publish(entry *ProgressEntry)
    Active() []ProgressEntry
}
```

### `Orchestrator` / `RateLimiter`

```go
type Orchestrator interface {
    Submit(job func(ctx context.Context) error) bool
    Shutdown()
}

type RateLimiter interface {
    Wait(ctx context.Context, n int64) error
    WaitFor(ctx context.Context, n int64, timeout time.Duration) error
}
```

---

## Migration Plan

总计 8 个 phase（含 Phase -1 目录重组），~7-10 天。

### Phase -1: 代码目录重组（半天，可与 Phase 0 并行）

纯文件 rename + import 替换，不动业务逻辑。

1. **批量 rename**（gofmt + sed 替换 import）：
   - `internal/crypt/` → `internal/cipher/`
   - `internal/drive/` → `internal/backend/`
   - `internal/cache/` → `internal/index/`
   - `internal/fs/` → `internal/fusefs/`
   - `internal/log/` → `internal/logging/`
2. **合并** `internal/sync/` + `internal/staging/` → `internal/upload/`
3. **新建** `mobile/` 占位目录
4. **`cmd/qrypt/{core,platform}.go` 移到** `internal/platform/`
5. **新建** `internal/mount/`、`internal/rpc/` 空目录（Phase 1-2 填充）

**验收：**
- `go build ./...` 通过
- 所有 e2e 测试不退化
- `git diff --stat` 仅有 rename + import 替换

### Phase 0: 基础（半天）

1. 删除 `core/qrypt/` 中所有 `Noop*` 默认实现
2. 删除 `NetworkConfig`/`BackgroundTask`/`AppLifecycle` 接口
3. 改造 `FileAPI` 接受 `Cipher` interface（不是 `*cipher.RcloneCipher`）
4. 在 `internal/platform/adapter.go` 中新增 `CipherAdapter` 包装 `*cipher.RcloneCipher`
5. CI 加 forbidden import 检查

**验收：**
- `go build ./core/qrypt/ ./cmd/qrypt/` 通过
- `grep -r "yinzhenyu/skills/qrypt/internal" core/qrypt/` 无输出
- `grep -r "Noop" core/qrypt/` 无业务 `Noop*`（仅测试 helper 允许）

### Phase 1: Session/Event/Progress/RateLimiter/Orchestrator 迁移（1 天）

```bash
rm internal/daemon/session.go
rm internal/daemon/events.go
rm internal/daemon/progress.go
rm internal/daemon/ratelimit.go
rm internal/daemon/orchestrator.go
```

更新 `internal/daemon/service.go`、`internal/mount/manager.go`、`internal/rpc/server.go` 等 import，改用 `core/qrypt` 对应类型。

**验收：**
- `internal/daemon/` 减 ~360 行
- FUSE mount + WS RPC 端到端可用

### Phase 2: CacheInvalidator 迁移 + MountLister 抽象（半天）

```go
// core/qrypt/cache.go 新增
type MountLister interface {
    ForEachRunningMount(fn func(name string) bool)
}
```

`internal/mount/manager.go` 的 `MountManager` 加 `ForEachRunningMount` 方法。`internal/daemon/cache_invalidator.go` 删除。

**验收：**
- 上传完成后 FUSE 节点树缓存正确失效
- 现有 `daemon_test.go`、`mount_manager_test.go` 不退化

### Phase 3: daemon/ 拆分为 4 个包 + service.go 拆分（1.5-2 天）

**Part A: 拆分 daemon/ 包**（与 Phase 1 顺接）

| 原 `internal/daemon/` 文件 | 拆到 |
|---------------------------|------|
| `daemon.go` / `socket.go` / `api.go` | `internal/daemon/`（lifecycle，~200 行）|
| `mount_manager.go` / `mount_fuse.go` / `mount.go` / `mount_nofuse.go` / `instance.go` | `internal/mount/` |
| `ws_server.go` / `ws_client.go` / `dispatch_test.go` | `internal/rpc/` |
| `service.go` 中 FUSE 节点树更新部分 | `internal/fusefs/`（与 FUSE 回调同包）|

**Part B: 拆分 `daemon/service.go` (1278 行)**

| 原方法 | 迁移目标 |
|--------|---------|
| `ListDir` / `Mkdir` / `Remove` / `Move` / `PushStart` / `PullStart` / `Find` / `Stat` | **删除**，由 `FileAPI` 取代 |
| `updateFuseNodes` / `syncTimer` / mount 状态机 | 保留 `internal/daemon/` 或迁入 `internal/fusefs/` |
| WS RPC handler 内部实现 | 改调 `FileAPI` |

预期 `service.go` 降到 ~500 行，`internal/daemon/` 总文件数从 20 降到 ~5。

**验收：**
- 所有 WS RPC 端到端通过 e2e
- `internal/daemon/service.go` 减少 ≥700 行
- `internal/daemon/` 仅保留 lifecycle 文件

### Phase 4: 单元测试（1-1.5 天）

`core/qrypt/*_test.go`：

- `mockDriver` / `mockWriter` / `mockUploader` / `mockPathResolver` / `mockCipher` 实现
- `FileAPI` 全部方法覆盖：
  - `List` / `Stat` / `Find` / `Mkdir` / `Move` / `Remove` / `Read` / `Push` / `Pull`
  - Move 跨父目录 vs 同父目录
  - Remove recursive vs 非 recursive
  - Read header + body 两阶段
- `SessionManager.Acquire` / `Release` 并发安全（`-race`）
- `Orchestrator.Submit` / `Shutdown` worker 生命周期
- `EventBus.Publish` 多订阅者
- `RateLimiter.Wait` 0 / 正常 / 超时
- `CacheInvalidator` 触发 `MountLister.ForEachRunningMount`

**验收：**
- `go test -race ./core/qrypt/` 通过
- FileAPI 覆盖率 100%，其他 ≥80%

### Phase 5: gomobile 验证（半天）

```bash
gomobile bind -target android -o /tmp/libqrypt.aar ./mobile/
```

仅验证 `mobile/` 入口的 gomobile 编译可行性。MobileAPI 的具体形状留到真做移动端时设计。

**验收：**
- `libqrypt.aar` 生成成功
- 无 MobileAPI 业务逻辑

### Phase 6: 清理（半天）

- 删除 `core/qrypt/` 死代码
- 统一命名（`PathResolver` 是否需要、`NewFileAPI` Options 字段顺序）
- 更新 README + AGENTS.md

---

## Testing Strategy

### 单元测试

- **必须**所有导出符号有测试
- mock 优先，不接真实网络/云盘
- CI 强制 `go test -race -coverprofile=coverage.out`

### 集成测试

保留 `cmd/qrypt/e2e_test.go`，作为冒烟测试。不强求覆盖率。

### CI 检查清单

```yaml
steps:
  - go build ./core/qrypt/
  - go build ./cmd/qrypt/
  - go vet ./...
  - go test -race -coverprofile=coverage.out ./core/qrypt/   # 新增
  - grep -r "yinzhenyu/skills/qrypt/internal" core/qrypt/   # 新增，期望无输出
  - go test ./cmd/qrypt/  # 现有 e2e
```

---

## Risks

| 风险 | 缓解 |
|------|------|
| 迁移过程中 FUSE 回归 | 每个 phase 跑 `qrypt mount` + e2e；逐 phase 提交可回滚 |
| `daemon/service.go` 拆分破坏 WS 协议 | 保留 `protocol` 包类型不变；handler 内部实现切到 FileAPI，外部接口稳定 |
| `MountLister` 抽象不够 | 当前只需 mount name，扩展接口时增加字段 |
| gomobile bind 失败 | Phase 5 才做，前 4 phase 不依赖 gomobile |
| `core/` 测试发现 `daemon/` 有 bug | 记录在 phase 4，迁移时一并修 |
| Cipher adapter 性能损失 | interface dispatch 在 cipher 不在热路径，可忽略 |
| v2 砍掉 3 个 Platform Services 后移动端不够用 | 移动端真做时按平台 API 重设计 1-3 个新接口，不破坏现有 API |
| 测试覆盖率不达标 | Phase 4 强制 80%，低于该值的组件标记 TODO |
| `NewFileAPI` 必传参数在 CLI 中传错 | 桌面 CLI 的 `internal/platform/adapter.go` 集中提供 4 个必传 impl，单一来源 |
| **Phase -1 rename 影响面大** | 仅文件 rename + import 替换，纯机械操作；每个 rename 后立即 `go build` 验证 |
| **拆 daemon/ 后 mount/rpc 包间循环依赖** | 依赖方向 `daemon → {mount, rpc} → core/qrypt`，三者不互相 import；先在 Phase 3 画出依赖图确认无环 |
| **`internal/log` 与 stdlib `log` 同包名引发隐式冲突** | Phase -1 rename 为 `internal/logging/` 解决 |

---

## 与 v1 的差异摘要

| 维度 | v1 | v2 |
|------|----|----|
| 核心叙事 | 提取跨平台库给未来移动端用 | `core/` 是 qrypt kernel，所有平台是消费者 |
| `core/` import `internal/` | 建议避免（实际触线） | **CI 强制禁止** |
| `Cipher` 接口 | 定义但未用（死代码） | 必须实现 + 测试 |
| `daemon/` 与 `core/` | 平行实现 | 真迁移，daemon/ 不持有 kernel 概念 |
| `daemon/service.go` | 1278 行保留 | 拆分到 ~500 行，文件操作全删 |
| Platform Services | 5 个 | 2 个（DirResolver + CredentialStore） |
| `Noop*` 默认 | 5 个 | 0 个（nil 返回错误） |
| 单元测试 | 目标但未强制 | 验收硬指标，CI 强制 |
| `FileAPI` 接受 | `Driver` + `*crypt.RcloneCipher` | `DriverFactory` + `Cipher` interface |
| gomobile 兼容 | build tag 切换 | 不在 `core/`，由 `mobile/` 包处理 |
| 事件类型 | `Data interface{}` | `Progress *ProgressEntry`（具体类型） |
| **代码目录** | 7 个内部包，命名混乱，`daemon/` 是 god package | 12 个内部分包，命名规范化，`daemon/` 拆为 4 个关注点包 |
| **`daemon/` 行数** | 3855 行 / 20 文件 | ~500 行 / 5 文件（lifecycle only） |
| **包命名** | `crypt`/`log` 与 stdlib 易混 | `cipher`/`logging` 明确区分 |
| **重复关注点** | `sync/` + `staging/` 分两个包 | 合并为 `upload/` |
| **WS RPC 范围** | 文件操作 + 控制平面 | 仅控制平面（mount/status/dashboard），文件操作走 FileAPI |
| **移动端接入** | 未明确入口 | `mobile/` 独立包，gomobile 边界清晰 |
