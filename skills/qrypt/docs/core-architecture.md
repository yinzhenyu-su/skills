# Qrypt Core: 跨平台核心库架构设计

## Context

qrypt 当前是单二进制架构，所有功能打包进一个 CLI，通过 build tag `nofuse` 排除 FUSE 相关代码：

```
cmd/qrypt/         ← CLI entry (cobra)
internal/
├── daemon/        ← 后台服务 + mount 管理 + IPC（~3500 行，20 文件）
├── fs/            ← FUSE filesystem (build tag: !nofuse)
├── drive/         ← 云盘驱动抽象 + 实现
├── crypt/         ← 加密引擎
├── sync/          ← 上传/下载管道
├── cache/         ← 缓存管理
├── staging/       ← 写入暂存区
├── config/        ← 配置管理
├── protocol/      ← IPC 协议
└── log/           ← 日志
```

这种结构的问题：

- **混编单元 vs 复用单元不一致**：`daemon/`、`protocol/`、`fs/` 是桌面/macOS FUSE 场景的特有编排层，对 Android/iOS 无意义。但 `drive/`、`crypt/`、`sync/`、`cache/`、`staging/` 是纯业务逻辑，平台无关。
- **复用需要靠手动复制或 build tag**：没有显式的库边界，Android/iOS 集成方案直接引用 `internal/` 包，缺乏稳定的 API surface。
- **测试成本高**：没有 `core` 包意味着移动端的功能验证必须跑完整集成测试。
- **平台服务无抽象**：目录路径、凭据存储、后台调度等平台相关行为散落在代码中，无法在移动端复用。

已有 `docs/mobile-integration.md` 和 `internal/daemon/mount_nofuse.go` + `internal/fs/fs_nofuse.go` 占位，但缺少一次正式的边界提取。

### 当前 daemon 职责分析

`internal/daemon/` 是控制平面，20 个文件 ~3500 行，7 块功能：

| 功能 | 文件 | 行数 | 平台依赖 | 是否可进 core |
|------|------|------|---------|-------------|
| FUSE 挂载生命周期 | `mount_fuse.go`, `mount_manager.go` | 506 | macOS/Linux FUSE | ❌ 平台特有 |
| RPC 服务端 (WebSocket) | `ws_server.go`, `api.go` | 740 | Unix socket IPC | ❌ 桌面 IPC 协议 |
| RPC 客户端 | `ws_client.go` | 172 | Unix socket IPC | ❌ 桌面 IPC 协议 |
| **Session 连接池** | `session.go` | 105 | 无 | ✅ 移入 core |
| **异步传输引擎** | `orchestrator.go` | 74 | 无 | ✅ 移入 core |
| **事件系统** | `events.go`, `progress.go` | 135 | 无 | ✅ 移入 core |
| **缓存一致性** | `cache_invalidator.go` | 81 | 持有 `*MountManager` | ⚠️ 需先提取 `MountLister` 接口 |
| 限流器 | `ratelimit.go` | 44 | 无 | ✅ 移入 core |
| **高层文件操作** | `service.go` | 1278 | 无 | ✅ 移入 core (FileAPI) |

**关键发现**：daemon 中 ~950 行可移入 core/（SessionManager、Orchestrator、事件系统、限流器 + service.go 中的高层文件操作方法）。CacheInvalidator（81行）持有 `*MountManager` 引用，需要先抽象 `MountLister` 接口才能移入。剩下 ~2150 行（FUSE mount 85 + mount_manager 421 + WS server 695 + WS client 172 + service.go 中剩余部分 + socket + api + cache_invalidator）是桌面平台特有的。

注：mount_manager.go（421行）并非完全是 FUSE 平台的——其 mount 启停逻辑通过 `mountBackend` 接口与 FUSE 解耦，但 `MountInstance` 中持有 `*fs.QryptFS` 和 `*fuse.FileSystemHost` 等 FUSE 类型，需要剥离后才能复用。mount_manager 的 `List()` / `Get()` / `LookupByPath()` 等查询方法（约100行）可以提取接口供 CacheInvalidator 使用。

### 目标平台能力差异

| 能力 | macOS | Linux | Windows | iOS | Android |
|------|-------|-------|---------|-----|---------|
| FUSE 挂载 | macFUSE (需安装) | libfuse/fuse3 | WinFsp (需安装) | ❌ | ❌ |
| 持久后台进程 | LaunchAgent | systemd user | Windows Service | ❌ (BGTaskScheduler 有限) | ForegroundService |
| 原生 App 技术栈 | Swift/SwiftUI | GTK/Qt/Electron | C#/WinUI | Swift/SwiftUI | Kotlin/Jetpack |
| Go 集成方式 | 嵌入/子进程 | 直接运行 | 直接运行 | gomobile .xcframework | gomobile .aar |
| 安全凭据存储 | Keychain | libsecret | DPAPI | Keychain | Keystore |
| 文件系统沙箱 | 无 | 无 | 无 | 严格沙箱 | app 私有目录 |
| 后台可被 Kill | ❌ | ❌ | ❌ | ✅ (频繁) | ✅ (频繁) |

## End State（一句话）

**拆分后的项目结构，每新增一个平台只需写 `<500` 行胶水代码（Platform Services 实现 + UI glue），0 行重复业务逻辑。**

### Before vs After

```
Before:                              After:
qrypt/                               qrypt/
├── cmd/qrypt/      (all-in-one)     ├── core/qrypt/     ← NEW: ~2000 行纯 Go 库
├── internal/                        │   ├── api.go      (FileAPI)
│   ├── daemon/    (3500行)           │   ├── mobile.go   (MobileAPI for gomobile)
│   ├── fs/        (3700行)           │   ├── drive.go    (Driver 接口定义)
│   ├── drive/                        │   ├── platform.go (DirResolver 等接口)
│   ├── crypt/                        │   ├── session.go  (从 daemon/ 迁入)
│   └── ...                           │   ├── upload.go   (从 daemon/ 迁入)
│                                    │   └── events.go   (从 daemon/ 迁入)
│                                    ├── internal/
│                                    │   ├── daemon/     ← 瘦身: ~2100 行 (移除 ~950 行)
│                                    │   ├── fs/         ← 不变: ~3700 行
│                                    │   ├── drive/      ← 不变
│                                    │   ├── crypt/      ← 不变
│                                    │   └── sync/       ← 不变
│                                    ├── cmd/qrypt/     ← 引用 core/qrypt/ 替代 daemon/service.go
│                                    ├── android/       ← NEW: <500 行 Kotlin + MobileAPI
│                                    └── ios/           ← NEW: <500 行 Swift + MobileAPI
```

### 验收标准

| 指标 | 目标 | 验证方式 |
|------|------|---------|
| **代码复用率** | 所有业务逻辑在 `core/` 中，零重复 | `core/` 不依赖 `fs/`、`daemon/` |
| **新增平台成本** | ≤500 行胶水代码 | 实现 Android bridge 后统计 |
| **构建产物** | CLI + AAR + XCFramework 三路并行 | CI 同时产出 |
| **测试覆盖** | `core/` 在任意平台 `go test` 通过 | CI 在 macOS + Linux 运行 |
| **daemon 瘦身** | `daemon/` 移除 ≥900 行 | `git diff --stat` |

## Goals（过程目标）

- 提取 `core/qrypt/` 作为正式的、平台无关的 Go 库，5 个平台共享同一份业务逻辑
- 定义 `core/qrypt.FileAPI` 作为顶层文件操作接口，替代 `daemon/service.go` 中的散装方法
- 定义 `core/qrypt.PlatformServices` 接口族，各平台注入自己的实现
- 复用率：不修改一行 `drive/`、`crypt/`、`sync/`、`cache/`、`staging/` 代码
- 测试：`core/` 的单元测试不依赖任何平台运行时，可在任意平台 `go test`

## Non-Goals

- 不重写 `drive/`/`crypt/`/`sync/`/`cache/`/`staging/` 内部逻辑
- 不改变现有 CLI 命令和配置格式
- 不做任何平台的完整 UI 实现（本设计只定义边界）
- 不引入依赖注入框架（保持 Go 风格的手动注入）
- 不改变 `daemon/` 现有 IPC 协议（向后兼容）

## Architecture

### 目标架构

```
┌──────────────────────────────────────────────────────────────────┐
│                    core/qrypt/ (纯 Go 库)                         │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │  FileAPI ── 高层文件操作（文件粒度）                       │  │
│  │  List / Read / Write / Mkdir / Move / Remove / Stat       │  │
│  │  Push / Pull                                               │  │
│  └──────────────────┬─────────────────────────────────────────┘  │
│                     │ 组合                                       │
│  ┌──────────────────▼─────────────────────────────────────────┐  │
│  │  Core Services (从 daemon/ 移入)                           │  │
│  │  · SessionManager (drive 连接池)                           │  │
│  │  · UploadQueue (worker pool)                               │  │
│  │  · EventManager (pub/sub)                                  │  │
│  │  · ProgressHub (传输进度)                                  │  │
│  │  · CacheInvalidator (缓存一致性)                           │  │
│  │  · RateLimiter                                              │  │
│  └──────────────────┬─────────────────────────────────────────┘  │
│                     │ 依赖                                       │
│  ┌──────────────────▼─────────────────────────────────────────┐  │
│  │  Platform Services 接口族（各平台注入实现）                │  │
│  │  · DirResolver      ─  路径约定                            │  │
│  │  · CredentialStore  ─  安全凭据存储                        │  │
│  │  · NetworkConfig    ─  网络配置（代理/证书）               │  │
│  │  · BackgroundTask   ─  后台任务调度                        │  │
│  │  · AppLifecycle     ─  生命周期通知                        │  │
│  └──────────────────┬─────────────────────────────────────────┘  │
│                     │ 依赖                                       │
│  ┌──────┬───▼───┬──────┬───────┬─────────┬──────────┐         │
│  │drive │ crypt │ sync │ cache │ staging │  config  │         │
│  └──────┴───────┴──────┴───────┴─────────┴──────────┘         │
│  (内部包，代码不动，仅移动位置或引用)                           │
└──────────────────────────────────────────────────────────────────┘
     ▲               ▲              ▲               ▲
     │ gomobile      │ gomobile     │ 直接运行       │ 嵌入/子进程
     │ .aar          │ .xcframework │               │
 ┌───┴───────┐ ┌───┴────────┐ ┌───┴──────┐ ┌──────┴─────────┐
 │ Android   │ │ iOS / macOS│ │ Linux    │ │ macOS / Windows │
 │ App       │ │ App        │ │ Desktop  │ │ Desktop (FUSE)  │
 │ (Kotlin)  │ │ (Swift)    │ │          │ │ ┌─────────────┐ │
 │           │ │            │ │          │ │ │ daemon/     │ │
 │           │ │            │ │          │ │ │ fs/         │ │
 └───────────┘ └────────────┘ └──────────┘ │ └─────────────┘ │
                                           └─────────────────┘
```

### 依赖方向

```
core/qrypt/  ─depends_on──►  internal/drive/    (通过接口)
  (纯 Go)                      internal/crypt/
                              internal/sync/
                              internal/cache/
                              internal/staging/
                              internal/config/
                              （所有依赖都是同一 Go module 内的 internal）

cmd/qrypt/    ─depends_on──►  core/qrypt/  +  internal/daemon/  +  internal/fs/
  (macOS/Linux/Windows CLI)

mobile/       ─depends_on──►  core/qrypt/   (via gomobile)
  (Android/iOS)

internal/     ◄── NO REVERSE DEPENDENCY ──
```

核心原则：**`core/` 不依赖任何平台特有包（`fs/`、`daemon/`、`protocol/`）**。

## Decisions

### D1: FileAPI — 以文件操作为粒度，非 FUSE 操作

```go
// core/qrypt/api.go
package qrypt

type FileAPI struct {
    drv   drive.Driver
    cp    *crypt.RcloneCipher
    sync  *sync.Pipeline
    cache *cache.CacheManager
}

// List 列出路径下的文件条目（解密文件名后返回）
func (a *FileAPI) List(ctx context.Context, mount, path string) ([]FileEntry, error)

// Read 读取文件全部或部分内容（自动解密）
func (a *FileAPI) Read(ctx context.Context, mount, path string, offset, size int64) (io.ReadCloser, error)

// Write 写入文件内容（暂存到 staging，异步加密上传）
func (a *FileAPI) Write(ctx context.Context, mount, path string, body io.Reader) error

// Mkdir 创建目录（加密目录名后调用云端 API）
func (a *FileAPI) Mkdir(ctx context.Context, mount, path string) error

// Move 移动/重命名
func (a *FileAPI) Move(ctx context.Context, mount, oldPath, newPath string) error

// Remove 删除文件或目录
func (a *FileAPI) Remove(ctx context.Context, mount, path string) error

// Stat 获取文件元数据
func (a *FileAPI) Stat(ctx context.Context, mount, path string) (*FileEntry, error)

// Push 将本地文件同步到云端（按路径）
func (a *FileAPI) Push(ctx context.Context, mount, localPath, remotePath string) error

// Pull 将云端文件下载到本地
func (a *FileAPI) Pull(ctx context.Context, mount, remotePath, localPath string) error
```

**理由**：这些操作覆盖了当前 `daemon/service.go` 中所有 CLI 命令（`ls`/`cat`/`push`/`pull`/`mv`/`rm`/`mkdir`/`find`/`status`）的核心逻辑。FUSE 场景的 `Read()`/`Write()` 以 block 为粒度，但 `FileAPI` 在文件级操作更通用，Android 不需要 block 级随机访问。

**FUSE 与 FileAPI 的关系**：FUSE 的 Read/Write 在内部拆解为 block 操作后调用 `drive.Reader.Read()` 和 `staging.Store.WriteAt()`，而不是通过 `FileAPI`。FUSE 仍然直接使用下层接口。`FileAPI` 是 CLI/Android 的高层接口。

### D2: core/ 放在项目根目录

```
qrypt/
├── core/                   ← 新建：平台无关核心库
│   └── qrypt/
│       ├── api.go          ← FileAPI
│       ├── api_test.go
│       ├── filename.go     ← FilenameResolver
│       ├── session.go      ← SessionManager（从 daemon/ 移入）
│       └── upload.go       ← UploadQueue（从 daemon/ 移入）
├── internal/
│   ├── drive/              ← 不动
│   ├── crypt/              ← 不动
│   ├── sync/               ← 不动
│   ├── cache/              ← 不动
│   ├── staging/            ← 不动
│   ├── config/             ← 不动
│   ├── daemon/             ← 移除 SessionManager、UploadQueue 到 core/
│   ├── fs/                 ← 不动
│   ├── protocol/           ← 不动
│   └── log/                ← 不动
├── cmd/
│   └── qrypt/              ← 引用 core/qrypt/
├── android/                ← 新建：Android 项目入口
│   ├── bridge.go           ← gomobile 导出，包装 core/qrypt/
│   └── jniLibs/
├── docs/
│   ├── core-architecture.md ← 本文
│   └── mobile-integration.md
```

**理由**：

- Go module 顶层包名 `core/qrypt/` 在 `import "github.com/yinzhenyu/skills/qrypt/core/qrypt"` 时语义清晰
- gomobile 只导出 `core/qrypt/`，`internal/` 保持私有
- SessionManager 和 UploadQueue 是平台无关的逻辑（连接池 + worker pool），从 `daemon/` 移入 `core/` 后 macOS daemon 和 Android 都能用
- `internal/` 保持私有，不暴露给外部消费者

### D3: 核心接口在 core/ 内自建定义，不直接引用 internal/

Core 不能直接引用 `internal/` 中的 Go 接口定义。原因不是 Go 的 `internal` 规则限制（同 module 内 core/ 可以引用 internal/），而是 **gomobile 导出边界**和**依赖倒置**原则：

- gomobile 构建 `core/qrypt/` 时会打包其所有依赖，包括 `internal/drive/` → 但这本身没问题
- 真正的问题是：`core/qrypt/` 应该作为稳定的 API surface，不应跟 `internal/` 中的实现细节耦合
- 部分需要复用的接口定义在 build-tag 保护的文件中（如 `fs.UploadQueue` 定义在 `//go:build !nofuse` 的 `internal/fs/fs.go` 中）

因此 core/ 自建核心接口定义，`internal/` 中的实现去实现这些接口：

```go
// core/qrypt/drive.go  — 自建定义，不引用 internal/drive

// UploadQueue 异步上传任务队列（移自 internal/fs/fs.go:35，去掉 build tag 限制）
type UploadQueue interface {
    Submit(job func(ctx context.Context) error) bool
}
```

| Core 组件 | 使用的接口定义位置 | 实现来源 |
|-----------|-------------------|---------|
| `FileAPI.List` | `core/qrypt.Driver.List()` | `internal/drive/quark.QuarkDriver` |
| `FileAPI.Read` | `core/qrypt.Driver.Read()` | 同上 |
| `FileAPI.Mkdir` | `core/qrypt.Writer.Mkdir()` | 同上，需类型断言 |
| `SessionManager` | `core/qrypt.Driver` | `internal/drive/quark.QuarkDriver` |
| `UploadQueue` | `core/qrypt.UploadQueue` | `daemon/Orchestrator` 实现此接口 |

这样 `Orchestrator` 移入 core/ 时不再依赖 `internal/fs.UploadQueue`（它依赖 build tag），而是依赖 `core/qrypt.UploadQueue`。

### D4: Config 不下沉到 core

`core/qrypt/` 不直接依赖 TOML 配置。`FileAPI` 构造函数接收已解析好的对象：

```go
func NewFileAPI(
    drv drive.Driver,
    cp *crypt.RcloneCipher,
    cacheDir string,
    opts Options,
) *FileAPI
```

**理由**：

- 配置解析是平台相关的行为：macOS 从 `~/.qrypt/qrypt.toml` 加载，Android 从 `SharedPreferences` 或传入的 JSON 加载
- 保持 `core/` 纯净，不引入文件 I/O 假设
- `internal/config/` 仍可用于 macOS 端，Android 端走自己的配置路径

### D5: 可移入 core/ 的组件及前提条件

从 `daemon/` 移入 `core/` 的组件列表（**需先解决依赖关系**）：

| 组件 | 文件 | 行数 | 迁移前提条件 |
|------|------|------|-------------|
| **SessionManager** | `session.go` | 105 | 无。仅依赖 `config` + `drive/factory`，已解耦 |
| **UploadQueue (Orchestrator)** | `orchestrator.go` | 74 | `core/` 中自建 `UploadQueue` 接口定义（原在 `internal/fs/fs.go:35` 且带 build tag） |
| **EventManager** | `events.go` | 65 | 可保留 `protocol.Event` 引用，或提取为泛型接口 |
| **ProgressHub** | `progress.go` | 83 | 同 EventManager |
| **RateLimiter** | `ratelimit.go` | 44 | 无。仅依赖 `golang.org/x/time/rate` |
| **CacheInvalidator** | `cache_invalidator.go` | 81 | ⚠️ 持有 `*MountManager` 引用。需先提取 `MountLister` 接口（仅需要 `ForEachRunningMount` 和 `Get` 方法） |
| **FileAPI 高层操作** | `service.go` | ~500 | 提取 ListDir/Mkdir/Remove/Move/Push/Pull 方法（纯 `drive.Driver` + `crypt.Cipher` 操作） |

**SessionManager**（代码已验证，`daemon/session.go`）：
- 导入：仅 `config` + `crypto/sha256` + `drive` + `drive/factory`
- 不依赖 daemon 中任何其他类型
- 可以直接复制到 `core/qrypt/session.go`，无需改动

**UploadQueue（Orchestrator）**（代码已验证，`daemon/orchestrator.go`）：
- 导入：仅 `context` + `sync` + `internal/log`
- 结构性实现 `fs.UploadQueue`（隐式接口匹配，未显式 import fs）
- 迁移时：将 `UploadQueue` 接口定义从 `internal/fs/fs.go` 移至 `core/qrypt/`，`Orchestrator` 引用此接口

**CacheInvalidator（代码验证发现问题）**：
- 导入包：`protocol` + `internal/log`（技术上可移入 core/）
- **但**结构体持有 `*MountManager`（`mount_manager.go` 中的类型）：
  ```go
  type CacheInvalidator struct {
      manager *MountManager
  }
  ```
- `MountManager` 是 daemon 特有的类型。解决方案：提取 `MountLister` 接口：
  ```go
  type MountLister interface {
      ForEachRunningMount(fn func(name string, inst *MountInstance) bool)
      Get(name string) (*MountInstance, error)
  }
  ```
  但这又把 `MountInstance` 带了进来……更好的方案是只定义 core 需要的回调：
  ```go
  type CacheInvalidatorHooks interface {
      InvalidateDirCache(mountName, dirPath string) error
  }
  ```
  让 daemon/ 实现此接口并注入 core/。

迁移后 `daemon/` 仍然持有这些组件的包装（例如通过 WebSocket 暴露队列状态），但核心逻辑在 `core/` 中。

### D6: Error 类型统一

```go
// core/qrypt/errors.go
package qrypt

type ErrorKind int

const (
    ErrNotFound      ErrorKind = iota // 文件/目录不存在
    ErrAlreadyExists                   // 已存在
    ErrNotEmpty                        // 目录非空
    ErrPermission                      // 权限不足（云端 cookie 过期等）
    ErrNetwork                         // 网络错误
    ErrInternal                        // 内部错误
)

type Error struct {
    Kind    ErrorKind
    Message string
    Cause   error
}
```

`FileAPI` 所有方法返回 `*Error`，调用方（macOS CLI / Android App）根据 `Error.Kind` 做本地化处理。

### D7: Platform Services 接口族

各平台差异通过接口抽象，由平台端注入实现：

```go
// core/qrypt/platform.go
package qrypt

// DirResolver 解析各平台的应用数据目录
type DirResolver interface {
    // CacheDir 缓存目录（可被系统清理，如 Android CacheDir / iOS Caches）
    CacheDir() string
    // DataDir 持久数据目录（用户数据，如 Android FilesDir / iOS Documents）
    DataDir() string
    // ConfigDir 配置文件目录（如 XDG_CONFIG_HOME / macOS Application Support）
    ConfigDir() string
}

// CredentialStore 安全凭据存取
// 各平台用原生机制实现：Keychain / Keystore / DPAPI / libsecret
type CredentialStore interface {
    Get(key string) (string, error)
    Set(key, value string) error
    Delete(key string) error
}

// NetworkConfig 网络配置（代理、证书等）
type NetworkConfig interface {
    // HTTPClient 返回平台适配的 HTTP 客户端
    // 桌面平台可注入系统代理设置，移动端可配置证书 pinning
    HTTPClient() *http.Client
}

// BackgroundTask 后台任务调度抽象
type BackgroundTask interface {
    // RunAsync 在后台执行 fn，返回后可查询状态
    RunAsync(ctx context.Context, id string, fn func(ctx context.Context) error) error
    // OnCompleted 任务完成回调（移动端用于恢复 UI）
    OnCompleted(id string) <-chan error
}

// AppLifecycle 生命周期通知
// 移动端 App 可能随时被 Kill，core 需要感知以做 checkpoint
type AppLifecycle interface {
    // Suspended 返回一个 channel，App 进入后台时关闭
    Suspended() <-chan struct{}
    // Resumed 返回一个 channel，App 回到前台时关闭
    Resumed() <-chan struct{}
}
```

**理由**：

- 这些是跨平台时唯一无法用纯 Go 统一处理的行为
- 接口设计为「最少方法」原则，每个接口 1-3 个方法，平台端实现成本低
- `CredentialStore` 解决了当前 TOML 明文存 Cookie/Password 的安全问题
- `BackgroundTask` 和 `AppLifecycle` 让 `core/` 的异步操作可以感知移动端生命周期

每个平台的实现方案（⚠️ 标记的表示依赖库尚未引入 go.mod，需要后续添加）：

| 接口 | macOS | iOS | Android | Linux | Windows |
|------|-------|-----|---------|-------|---------|
| DirResolver | `NSSearchPathForDirectoriesInDomains` + `~/.qrypt` | `NSSearchPath` | `context.filesDir/cacheDir` | `$XDG_*` + `~/.qrypt` | `%APPDATA%` |
| CredentialStore | Keychain via `go-keychain` ⚠️ | Keychain via `go-keychain` ⚠️ | EncryptedSharedPreferences (Java/Kotlin 侧) | libsecret ⚠️ 或文件加密兜底 | DPAPI (syscall) ⚠️ |
| NetworkConfig | 系统代理 + 系统 CA | ATS 配置 (Native 侧) | Network Security Config (Native 侧) | 环境变量代理 | 环境变量代理 |
| BackgroundTask | LaunchAgent (Native 侧) | BGTaskScheduler (Native 侧) | WorkManager (Native 侧) | systemd timer (Native 侧) | Task Scheduler (Native 侧) |
| AppLifecycle | NSApp delegate | UIApplicationDelegate | Activity lifecycle | 无 | 无 |

Desktop CLI 过渡方案：在 go.mod 添加平台安全存储库之前，先用 `TomlCredentialStore`（加密文件存凭据）+ 提示用户迁移。

### D8: FileAPI 构造函数接收 Platform Services

```go
// core/qrypt/api.go

type FileAPI struct {
    drv      drive.Driver
    cp       *crypt.RcloneCipher
    dirs     DirResolver
    creds    CredentialStore
    netCfg   NetworkConfig
    bgTask   BackgroundTask
    lifecycle AppLifecycle
    // 内部组件
    sessions  *SessionManager
    uploadQ   *UploadQueue
    events    *EventManager
    progress  *ProgressHub
    cacheInv  *CacheInvalidator
    rateLimit *RateLimiter
}

func NewFileAPI(
    drv drive.Driver,
    cp *crypt.RcloneCipher,
    dirs DirResolver,
    creds CredentialStore,
    netCfg NetworkConfig,
    opts Options,
) *FileAPI {
    // 内部自动创建 SessionManager、UploadQueue、EventManager 等
    // cache、staging 目录通过 dirs.CacheDir() / dirs.DataDir() 获取
}
```

**理由**：

- SessionManager 的 driver 实例缓存需要 CredentialStore 来比较凭据
- CacheManager 和 staging 的数据目录需 DirResolver 提供
- UploadQueue 的 worker 生命周期需 AppLifecycle 感知
- HTTP client 从 NetworkConfig 获取以便注入代理或证书

### D9: API 签名适配 gomobile 导出限制

gomobile 限制：**不能导出** `io.Reader`、`interface{}`、`chan`、`func` 参数。

FileAPI 针对移动端提供导出变体：

```go
// core/qrypt/mobile.go  (build tag: gomobile)

package qrypt

// MobileAPI 是 gomobile 友好的 FileAPI 变体，所有参数和返回值都是可导出类型
type MobileAPI struct {
    inner *FileAPI
}

// List 返回 JSON 字符串（gomobile 不支持复杂结构切片）
func (m *MobileAPI) List(ctx, mount, path string) (string, error)

// ReadData 读取完整文件内容到 byte slice（gomobile 不支持 io.Reader）
func (m *MobileAPI) ReadData(ctx, mount, path string) ([]byte, error)

// WriteData 从 byte slice 写入文件
func (m *MobileAPI) WriteData(ctx, mount, path string, data []byte) error

// PushFile 上传本地文件（按路径）
func (m *MobileAPI) PushFile(ctx, mount, localPath, remotePath string) (string, error) // 返回 taskID

// PullFile 下载到本地文件
func (m *MobileAPI) PullFile(ctx, mount, remotePath, localPath string) (string, error) // 返回 taskID

// StatusJSON 返回聚合状态（JSON 字符串）
func (m *MobileAPI) StatusJSON() string
```

桌面端（CLI / FUSE）继续用 `FileAPI` 的完整签名（`io.ReadCloser`、`[]FileEntry` 等）。

**大文件流式传输**：MobileAPI 不直接支持 streaming。iOS/Android 端大文件场景走临时文件：

```
远程文件 → PullFile(tmpPath) → 原生端 mmap/流式读取 tmpPath
本地文件 → 写入 tmpPath → PushFile(tmpPath, remotePath)
```

**理由**：

- gomobile 不支持 `io.Reader` 跨语言边界传递，临时文件是最通用的折中方案
- 移动端典型场景是照片/视频上传（几 MB 到几百 MB），临时文件开销可接受
- 桌面端不走 MobileAPI，无 overhead

### D10: 文件访问模型 — 每平台一种模式

FUSE 不是通用方案。每个平台的最佳访问模型不同：

| 平台 | 访问模式 | 核心机制 | 对 core 的要求 |
|------|---------|---------|---------------|
| macOS | FUSE 挂载 | cgofuse → macFUSE | 直接调 `drive.Reader.Read()` + `crypt.DecryptBlock()` |
| macOS (无 FUSE) | Sync 目录 | `FileAPI.Push/Pull` + 本地 watcher | `FileAPI` + 平台端 fsnotify |
| Linux | FUSE 挂载 | cgofuse → libfuse | 同 macOS |
| Linux (无 FUSE) | Sync 目录 | `FileAPI.Push/Pull` + inotify | `FileAPI` + fsnotify |
| Windows | FUSE 或 Sync | WinFsp 或 `FileAPI.Push/Pull` | 同上 |
| iOS | 按需下载 | `MobileAPI.PullFile` + 本地缓存 | `MobileAPI`（FileAPI 变体） |
| Android | 按需下载 + SAF | `MobileAPI` + ContentProvider | `MobileAPI` |

**核心原则**：无论哪种访问模式，底层都是同一套 `drive + crypt + sync + cache`。`FileAPI` 是 Sync 模式的基础。FUSE 模式直接操作下层接口（绕过 FileAPI），性能更优但平台受限。

### D11: 凭据管理迁移路径

当前：Cookie 和 Password 明文存在 `qrypt.toml` 中。

目标：所有平台使用原生安全存储。

迁移策略：

| 阶段 | 行为 | 兼容性 |
|------|------|--------|
| 1. 现状 | TOML 明文存 Cookie/Password | 所有平台可用 |
| 2. core CredentialStore 接口 | FileAPI 支持从 CredentialStore 读取 | 桌面端可继续用 TOML 桥接（`TomlCredentialStore` 实现） |
| 3. 平台原生实现 | macOS Keychain / Android Keystore 等 | 各平台逐步切换，不破坏现有配置 |
| 4. TOML 降级 | TOML 中不再存储凭据，仅存路径引用（如 `credential_ref = "qrypt/main"`） | 向后兼容，旧配置自动迁移 |

```go
// core/qrypt/credential.go

// TomlCredentialStore 是迁移桥接实现：从 TOML Config 读取
// 用于桌面端切换到原生存储之前的过渡期
type TomlCredentialStore struct {
    cfg *config.Config
}
```

### D12: Build 策略

| 产物 | 构建方式 | 目标平台 | 内容 |
|------|---------|---------|------|
| `qrypt` CLI | `go build ./cmd/qrypt` | macOS, Linux, Windows | core + daemon + fs + CLI |
| `libqrypt_core.aar` | `gomobile bind -target android core/qrypt` | Android | core + MobileAPI |
| `QryptCore.xcframework` | `gomobile bind -target ios core/qrypt` | iOS | core + MobileAPI |
| `libqrypt_core.so` | `go build -buildmode=c-shared ./mobile/` | Android (JNI 方式) | core + 自定义 JNI bridge |

```bash
# macOS / Linux / Windows CLI
go build -o qrypt ./cmd/qrypt/

# Android AAR
gomobile bind -target android -o qrypt-core.aar \
  -androidapi 24 ./core/qrypt/

# iOS XCFramework
gomobile bind -target ios -o QryptCore.xcframework \
  ./core/qrypt/

# 注：gomobile bind 会自动忽略 internal/（因 internal 限制不可导出）
# core/qrypt/ 不应引用 internal/，或者 core/ 放在 module 根目录避免 internal 限制
```

**core/ 与 internal/ 的关系**：

技术上，`core/qrypt/` **可以** import `internal/drive/`——Go 的 `internal` 可见性规则只限制"外部"模块访问，`core/` 和 `internal/` 同属 `github.com/yinzhenyu/skills/qrypt` 模块，因此 `core/qrypt/` 可以引用 `internal/...`。gomobile 构建时也会将 `internal/` 作为依赖打包进 AAR/XCFramework。

但**设计上不推荐**这样做，原因是：

- `internal/drive/` 的接口背后绑定了 `internal/config.MountParams` 等配置类型，core/ 不应感知 TOML 配置结构
- `internal/` 中的接口设计面向 FUSE 场景（如 `drive.Reader.Read()` 返回 `io.ReadCloser`），未必是 core/ 消耗方的最佳抽象
- 保持 core/ 的 API surface 稳定，不受 internal/ 重构影响

因此**采用接口倒置**：`core/qrypt/` 中定义消费者视角的精简接口，具体实现由各端注入：

- `core/qrypt.Driver` — 精简自 `internal/drive.Driver`，只包含 core/ 需要的方法
- `core/qrypt.UploadQueue` — 移自 `internal/fs.UploadQueue`（原定义带 `nofuse` build tag，不适合 core/）
- 具体实现（`quark.QuarkDriver`、`Orchestrator` 等）在注入时做类型适配

```go
// core/qrypt/drive.go  —— 接口定义（非 internal/driver 的复制，而是精简后的消费者接口）
package qrypt

type Driver interface {
    List(ctx context.Context, parentID string) ([]Entry, error)
    Read(ctx context.Context, entry Entry, offset, size int64) (io.ReadCloser, error)
    // 可选接口：按需做类型断言
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
```

各平台端实现注入：

```
cmd/qrypt/  ──  internal/drive/quark.QuarkDriver 实现了 core/qrypt.Driver
                     └── 注入到 NewFileAPI(drv, ...)

mobile/      ──  internal/drive/quark.QuarkDriver 实现了 core/qrypt.Driver
                     └── 注入到 NewFileAPI(drv, ...)（编译时由 gomobile 打包）
```

## Migration Plan

### Phase 0: daemon 职责盘点与边界划定 (0.5d)

1. 确认 `daemon/` 中哪些方法可以移入 `core/` 的 `FileAPI`（service.go 中的 ListDir、Mkdir、Remove、Move、PushStart、PullStart 等）
2. 确认 `daemon/` 中哪些组件可以移入 `core/` 作为 Core Services（session.go、orchestrator.go、events.go、progress.go、ratelimit.go、cache_invalidator.go）
3. 标记 `daemon/` 中依赖 FUSE/Unix socket 的代码（mount_fuse.go、ws_server.go、ws_client.go）

### Phase 1: 定义 core/qrypt/ 接口 (0.5d)

1. 定义 `core/qrypt/drive.go`（Driver / Writer / Uploader 接口，精简自 `internal/drive`）
2. 定义 `core/qrypt/platform.go`（DirResolver / CredentialStore / NetworkConfig / BackgroundTask / AppLifecycle）
3. 定义 `core/qrypt/errors.go`（ErrorKind 统一错误类型）
4. 定义 `core/qrypt/types.go`（FileEntry 等公共类型）

### Phase 2: 实现 core/qrypt/ Core Services (1d)

1. 从 `daemon/session.go` 迁移 SessionManager（改为依赖 `core/qrypt.Driver` + `core/qrypt.CredentialStore`）
2. 从 `daemon/orchestrator.go` 迁移 UploadQueue
3. 从 `daemon/events.go` 迁移 EventManager
4. 从 `daemon/progress.go` 迁移 ProgressHub
5. 从 `daemon/cache_invalidator.go` 迁移 CacheInvalidator
6. 从 `daemon/ratelimit.go` 迁移 RateLimiter

### Phase 3: 实现 FileAPI (1.5d)

1. 实现 `NewFileAPI` 构造函数（注入 Driver + Cipher + Platform Services）
2. 实现 `List`（复用 Driver.List + cipher.DecryptSegment）
3. 实现 `Stat`（复用 Driver.List 单条目查询）
4. 实现 `Mkdir` / `Move` / `Remove`（复用 Writer 接口 + cipher.EncryptSegment）
5. 实现 `Push` / `Pull`（复用 sync.Uploader / sync.Downloader）
6. 实现 `Read` / `Write`（文件级，临时文件 + staging 中转）
7. 实现 `MobileAPI`（gomobile 友好包装）

### Phase 4: 平台端实现 Platform Services (各 0.5d)

| 平台 | 需要实现的接口 | 难度 |
|------|-------------|------|
| macOS CLI | TomlDirResolver + TomlCredentialStore + DefaultNetworkConfig + NoopBackgroundTask + NoopAppLifecycle | 低 |
| macOS App | DirResolver via NSFileManager + KeychainCredentialStore + DefaultNetworkConfig | 中 |
| iOS | DirResolver via NSSearchPath + KeychainCredentialStore + ATS-aware NetworkConfig + BGTaskScheduler BackgroundTask + UIApplicationLifecycle | 高 |
| Android | DirResolver via Context + EncryptedSharedPrefsCredentialStore + WorkManager BackgroundTask + ActivityLifecycle | 高 |
| Linux CLI | XDG DirResolver + libsecret CredentialStore + DefaultNetworkConfig | 中 |
| Windows CLI | APPDATA DirResolver + DPAPI CredentialStore + WinHTTP NetworkConfig | 中 |

### Phase 5: 桌面 CLI 切换 (1d)

1. `cmd/qrypt/ls.go` → `FileAPI.List()`
2. `cmd/qrypt/cat.go` → `FileAPI.Read()`
3. `cmd/qrypt/push.go` → `FileAPI.Push()`
4. `cmd/qrypt/pull.go` → `FileAPI.Pull()`
5. `cmd/qrypt/mv.go` / `rm.go` / `mkdir.go` → 对应 FileAPI 方法
6. `daemon/service.go` 中重复方法标记 deprecated
7. daemon 的 WebSocket API 包装 core/ 中的实例，保持向后兼容

### Phase 6: 移动端集成 (各 1d)

1. Android: gomobile bind → .aar → Kotlin 调 MobileAPI
2. iOS: gomobile bind → .xcframework → Swift 调 MobileAPI
3. 实现各自的 Platform Services 注入
4. 实现 UI 层：文件浏览 / 上传下载 / 状态展示

## Risks / Trade-offs

| 风险 | 缓解措施 |
|------|---------|
| **SessionManager/UploadQueue 从 daemon/ 迁出后，daemon/ 的 WebSocket API 需要适配** | daemon/ 包装 core/ 中的实例，通过组合而非继承暴露相同方法。WebSocket 协议不感知底层变化 |
| **FileAPI.Read() 以文件为粒度，FUSE 的 block 级随机访问走不同路径** | 明确分层：`FileAPI` 面向 CLI/移动端（文件级），FUSE 直接调用 `drive.Reader.Read()` + `crypt.DecryptBlock()`，不经过 `FileAPI` |
| **gomobile 导出限制（无 io.Reader、interface{}、chan、func）** | `MobileAPI` 所有方法使用 `[]byte`、`string`、`error` 返回，大文件走临时文件中转 |
| **core/ 无法 import internal/ 导致接口定义重复** | 在 `core/qrypt/` 中定义核心接口（Driver/Writer/Uploader），`internal/drive/` 的具体实现实现这些接口。各端注入时做类型适配 |
| **FileAPI 的操作粒度不适合大文件随机读** | 大文件走 FUSE（桌面）或直接用 `drive.Reader.Read()` 自行解密（移动端）。`FileAPI.Pull()` 适用于完整文件传输 |
| **移动端 App 被 Kill 导致上传/下载中断** | 异步操作使用 journal（`pending.jsonl`）+ context 取消。重启后从 journal 恢复未完成任务 |
| **Platform Services 接口数过多增加实现负担** | 每个接口 1-3 个方法，提供默认实现（`DefaultDirResolver`、`NoopBackgroundTask`），平台端只需覆盖需要自定义的部分 |
| **gomobile 打包体积** | 仅 `core/qrypt/` 不包含 `fs/` 和 `daemon/`，预计 .aar/.xcframework 在 5-10MB 范围（Go runtime + net/http + crypto） |
| **iOS App Store 审核 — gomobile 使用动态库** | gomobile 生成的是静态链接的 .xcframework，不包含动态库，无审核风险 |
| **Windows FUSE 依赖 WinFsp 安装** | 提供两种模式：WinFsp 可选安装（FUSE 挂载），兜底使用 Sync 目录模式（FileAPI Push/Pull） |
| **多平台维护成本** | core/ 覆盖 90% 业务逻辑，平台端每平台 <500 行胶水代码（Platform Services 实现 + UI glue）。CI 覆盖 5 平台构建
