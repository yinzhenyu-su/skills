# Qrypt VFS Architecture v3

> 基于 `docs/core-architecture-v2.md` 的 VFS 层重构设计。本文件仅描述 FUSE 内部三层分解方案，不涉及 `core/qrypt/` 的移动端抽象。
>
> **讨论记录**: 该方案经 `/grill-me` 会话逐层分解设计，结论见 `docs/tasks/vfs-architecture-decisions.md`

## 设计目标

1. **FUSE 协议解耦**：VFS 接口纯 Go，不依赖 `cgofuse`，CLI 可复用
2. **加密透明**：`CipherDriver` 封装全部加解密，VFS 层不碰 `cipher.Cipher`
3. **缓存统一**：`CacheLayer` 管理 L1 memCache + L2 diskCache + 并发去重 + 预取
4. **写操作合并**：保留已有的 `OperationCoalescer`，将其输出路由到 `OpExecutor`
5. **写操作调度**：`UploadManager` 管理 writeBack 定时器 + 重试 + 通过 `OpExecutor` 执行

## 最终分层

```
┌──────────────────────────────────────────────────────────────┐
│                    FUSE 适配层 (fuse.go)                      │
│   cgofuse FileSystemBase 实现，纯参数/错误码转换              │
│   约 300 行薄层，无业务逻辑                                    │
├──────────────────────────────────────────────────────────────┤
│                         VFSImpl                              │
│  ┌────────┬──────────┬──────────┬──────────┬───────────────┐  │
│  │ Tree   │ Staging  │CacheLayer│CipherDrv │ UploadManager │  │
│  │ (内聚) │(明文缓存) │(读缓存)  │(加密层)  │(调度器+执行)  │  │
│  │        │          │          │          │               │  │
│  │ node   │ 创建/写入 │ L1 mem   │ ReadBlock│ writeBack     │  │
│  │ tree   │ 读取/截断 │ L2 disk  │ List     │ 重试          │  │
│  │ lookup │ 清理     │ dedup    │ Mkdir    │ OpExecutor    │  │
│  │ rename │          │ prefetch │ Remove   │ observer      │  │
│  │        │          │          │ Rename   │               │  │
│  │        │          │          │ Put      │               │  │
│  └────────┴──────────┴──────────┴──────────┴───────────────┘  │
│                             │                                 │
│                    ┌────────┴────────┐                        │
│                    │ OperationCoalescer│                       │
│                    │ （已有，合并删除/改名/mkdir）              │
│                    └────────┬────────┘                        │
│                             │                                 │
│                    ┌────────┴────────┐                        │
│                    │   OpExecutor    │                        │
│                    │ （写操作执行器） │                        │
│                    └────────┬────────┘                        │
├─────────────────────────────┼────────────────────────────────┤
│                    ┌────────┴────────┐                        │
│                    │  drivers.Driver │                        │
│                    │ quark/localfs   │                        │
│                    └─────────────────┘                        │
└──────────────────────────────────────────────────────────────┘
```

## 包目录结构

```
skills/qrypt/
│
├── pkg/
│   ├── vfs/                   ← VFS 接口（公开，纯 Go，无 FUSE 依赖）
│   │   └── vfs.go
│   │
│   └── cipherdriver/          ← CipherDriver（公开，透明的加密存储适配器）
│       └── cipher_driver.go
│
├── cipher/                    ← 加密算法
│   ├── rclone_cipher.go       ← 已有（RcloneCipher 实现）
│   ├── encrypt_reader.go      ← 从 core/qrypt/ 移入
│   ├── decrypt_reader.go      ← 从 core/qrypt/ 移入
│   └── encrypt_put.go         ← 从 core/qrypt/ 移入（可选）
│
├── drivers/                   ← 存储后端接口 + 实现（纯，不碰加密）
│   ├── driver.go              ← Driver / Writer / Uploader 接口
│   ├── registry.go            ← 驱动注册表
│   ├── quark/                 ← 夸克网盘实现
│   ├── localfs/               ← 本地文件系统实现
│   └── yun139/                ← 115 盘实现
│
├── internal/
│   │
│   ├── vfs/                   ← VFSImpl + 子管理器（全平台可编译，无 cgofuse 依赖）
│   │   ├── vfsimpl.go         ← VFS 接口实现、构造
│   │   ├── node.go            ← VFSNode
│   │   ├── tree.go            ← 树操作
│   │   ├── cache.go           ← CacheLayer
│   │   ├── upload.go          ← UploadManager
│   │   ├── executor.go        ← OpExecutor
│   │   ├── coalescer.go       ← OperationCoalescer（从 internal/fusefs 移入）
│   │   └── observer.go        ← UploadObserver 接口
│   │
│   ├── fusefs/                ← 薄 FUSE 适配层（仅此层需要 cgofuse + !nofuse build tag）
│   │   ├── fuse.go            ← FileSystemBase 实现，调 pkg/vfs.VFS 接口
│   │   ├── mount.go           ← mount options
│   │   ├── mount_darwin.go    ← macOS 挂载
│   │   ├── mount_linux.go     ← Linux 挂载
│   │   ├── unmount_darwin.go
│   │   ├── unmount_linux.go
│   │   └── fs_nofuse.go       ← nofuse 编译标签 stub
│   │
│   ├── mount/                 ← MountManager（已有，构造 VFSImpl 替代 QryptFS）
│   ├── config/                ← 配置（已有）
│   ├── logging/               ← 日志（已有）
│   ├── protocol/              ← JSON-RPC 类型（已有）
│   ├── rpc/                   ← JSON-RPC 服务（已有）
│   ├── platform/              ← 平台抽象（已有）
│   └── daemon/                ← 守护进程（已有）
│
├── core/qrypt/                ← 应用内核（减负后，加密 Reader 已移入 cipher/）
│   ├── cache.go               ← CacheManager（供 CacheLayer L2 diskCache 使用）
│   ├── staging.go             ← Store（供 VFSImpl staging 使用）
│   ├── api.go                 ← FileAPI
│   ├── events.go              ← EventBus
│   ├── session.go             ← SessionManager
│   ├── upload.go              ← Orchestrator
│   ├── platform.go            ← DirResolver / CredentialStore
│   └── chunk_cache.go         ← 块缓存索引
│
├── cmd/qrypt/                 ← CLI（已有）
│
└── docs/
    └── vfs-architecture.md    ← 本文
```

### 关键改动

| 包 | 改动 |
|---|---|
| `pkg/vfs/` | **新增**。公开 VFS 接口 |
| `pkg/cipherdriver/` | **新增**。CipherDriver 实现 `drivers.Driver/Writer/Uploader` |
| `cipher/` | **扩容**。从 `core/qrypt/` 移入 EncryptingReader / DecryptingReader |
| `internal/vfs/` | **新增**。VFSImpl + Tree + CacheLayer + UploadManager + OpExecutor + Coalescer |
| `internal/fusefs/` | **精简**。只保留 FUSE 适配层 + 挂载代码。无业务逻辑 |
| `internal/upload/` | **移除**。功能由 OpExecutor + CipherDriver.Put 替代 |
| `core/qrypt/` | **减负**。加密 Reader 移入 cipher/，其余不变 |
| `internal/vfs/` 测试 | 不依赖 cgofuse，macOS / Linux / Windows 全平台可运行 `go test` |

### 依赖方向

```
pkg/vfs           ← 无依赖（仅 Go 标准库）
pkg/cipherdriver  ← cipher.Cipher + drivers.Driver/Writer/Uploader
internal/vfs      ← pkg/vfs + pkg/cipherdriver + cipher + drivers + core/qrypt（staging + cacheMgr）
internal/fusefs   ← pkg/vfs + internal/vfs + cgofuse（FUSE 适配层，最上层）
```

## 层一：VFS 接口 (pkg/vfs/vfs.go)

```go
package vfs

import (
    "context"
    "io"
    "time"
)

type NodeInfo struct {
    ID      string    // fid（内部用）
    Name    string    // 明文文件名
    IsDir   bool
    Size    int64     // 明文大小
    ModTime time.Time
}

type DirEntry struct {
    Name  string
    IsDir bool
    Size  int64
}

type VFS interface {
    // 读路径
    Stat(ctx context.Context, path string) (NodeInfo, error)
    ListDir(ctx context.Context, path string) ([]DirEntry, error)
    Read(ctx context.Context, path string, offset int64) (io.ReadCloser, error)

    // 写路径
    Create(ctx context.Context, path string) error
    Write(ctx context.Context, path string, offset int64, reader io.Reader) error
    Mkdir(ctx context.Context, path string) error
    Remove(ctx context.Context, path string) error
    Rename(ctx context.Context, oldPath, newPath string) error
    Truncate(ctx context.Context, path string, size int64) error
    Utimens(ctx context.Context, path string, mtime time.Time) error

    // 同步
    Flush(ctx context.Context, path string) error
    Fsync(ctx context.Context, path string) error

    // 生命周期
    Shutdown()
}
```

### 设计决策

- **path-based**：入参全部用 path，FUSE 协议天然 path-based。`NodeInfo.ID` 暴露 fid 供内部使用。
- **Read 返回 `io.ReadCloser`**：避免大文件全量读入内存。FUSE 适配层通过 `io.ReadFull` 填满 FUSE buffer。
- **Write 接收 `io.Reader`**：对称设计，CLI 可流式写入。
- **无 Open/Release**：Qrypt 不需要文件 handle，每次 Read/Write 通过 path lookup。减少接口复杂度。
- **无 Chmod/Chown/xattr**：Quark 不支持。FUSE 适配层直接返回 lookup 确认存在。
- **Truncate**：VFS 需要此方法，因为实现需要 staging + MarkDirty，FUSE 适配层无法用 Read+Write 组合。

## 层二：CipherDriver (pkg/cipherdriver/cipher_driver.go)

```go
package drivers

import (
    "context"
    "sync"
    "github.com/yinzhenyu/skills/qrypt/cipher"
)

// CipherDriver wraps drivers.Driver with transparent encryption.
// 读路径：ReadBlock 返回明文块
// 写路径：Mkdir/Remove/Rename/Put 自动加解密文件名/内容
type CipherDriver struct {
    raw        Driver
    cp         cipher.Cipher
    nonceCache sync.Map // fid → [24]byte（nonce 为文件级参数，首次读取后缓存）
}

var (
    _ Driver    = (*CipherDriver)(nil)
    _ Writer    = (*CipherDriver)(nil)
    _ Uploader  = (*CipherDriver)(nil)
)

func NewCipherDriver(raw Driver, cp cipher.Cipher) *CipherDriver

// --- Read path ---

// ReadBlock 读取并解密第 blockIdx 个加密块，返回明文。
// blockIdx: 0-based 加密块索引（不含文件头）。
// 内部自动处理：
//   1. nonce 获取：首次读 block==0 时从文件头提取，后续缓存
//   2. 密文偏移换算：offset = FileHeaderSize + blockIdx * BlockSize
//   3. 解密：cp.DecryptBlock(ciphertext, blockIdx, nonce)
func (cd *CipherDriver) ReadBlock(ctx context.Context, entry Entry, blockIdx uint64) ([]byte, error)

// --- Driver interface ---

// List 返回目录下的 Entry 列表，已解密的文件名。
func (cd *CipherDriver) List(ctx context.Context, parentID string) ([]Entry, error)

// Read 透传原始 driver 的 Read（不解密，返回密文）。
func (cd *CipherDriver) Read(ctx context.Context, entry Entry, offset, size int64) (io.ReadCloser, error)

// Init/Drop 透传
func (cd *CipherDriver) Init(ctx context.Context) error
func (cd *CipherDriver) Drop(ctx context.Context) error

// --- Writer interface ---

func (cd *CipherDriver) Mkdir(ctx context.Context, parentID, name string) (Entry, error)
func (cd *CipherDriver) Move(ctx context.Context, entry Entry, dstParentID string) error
func (cd *CipherDriver) Rename(ctx context.Context, entry Entry, newName string) error
func (cd *CipherDriver) Remove(ctx context.Context, entry Entry) error

// Mkdir: 内部 cp.EncryptSegment(name) 后调 raw.Mkdir
// Rename: 内部 cp.EncryptSegment(newName) 后调 raw.Rename
// Remove: fid-based，不需加密，透传
// Move: 透传（目标目录 parentID 不加密）

// --- Uploader interface ---

// Put 接收明文文件名和明文 Reader，内部加密后上传。
// nonce 由 CipherDriver 内部生成，不暴露给调用方。
func (cd *CipherDriver) Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (Entry, error)
```

### 设计决策

- **`ReadBlock` 按块索引读取**：缓存粒度（`CacheLayer`）和预取粒度天然和块对齐，不需要字节偏移换算。
- **`Read` 透传密文**：`drivers.Driver.Read` 的约定是读密文，CipherDriver 不破坏此契约。
- **nonce 内部缓存**：崩溃后可重新从远程文件头提取，无状态丢失风险。
- **`Put` 内部生成 nonce**：上传是完整的"创建新文件"操作，不需要外部传 nonce。崩溃后整个文件重新上传。
- **`upload.Uploader` 不再需要 cipher**：上传层调 `CipherDriver.Put`，加密已在内。

## 层三：CacheLayer (internal/vfs/cache.go)

```go
package vfs

import (
    "context"
    "sync"
    "github.com/hashicorp/golang-lru/v2"
    "github.com/yinzhenyu/skills/qrypt/core/qrypt"
)

// FetchFunc 由 VFSImpl 注入，封装 CipherDriver.ReadBlock 调用
type FetchFunc func(ctx context.Context, fid string, blockIdx uint64) ([]byte, error)

type CacheLayer struct {
    memCache  *lru.Cache[string, []byte] // L1 内存 LRU：key = "fid_blockIdx"
    diskCache *qrypt.CacheManager        // L2 磁盘缓存：解密后的明文块
    dedup     sync.Map                   // 并发去重：key = "fid_blockIdx" → chan struct{}
    fetcher   FetchFunc                  // L3 远程获取（由 VFSImpl 注入）
    prefetch  *SeqReadTracker            // 顺序读检测 + 预取触发
}

func NewCacheLayer(memSize int, diskCache *qrypt.CacheManager, fetcher FetchFunc) *CacheLayer

// ReadBlock 返回解密后的明文块。
// 优先顺序：L1 memCache → L2 diskCache → L3 fetcher
// 并发保护：同块的并发请求只发一次 L3 请求，其余等待
func (cl *CacheLayer) ReadBlock(ctx context.Context, fid string, blockIdx uint64) ([]byte, error)

// NotifyRead 通知 CacheLayer 一次读事件，用于顺序读检测 + 触发预取
func (cl *CacheLayer) NotifyRead(fid string, blockIdx uint64)

// InvalidateFid 删除指定文件的所有缓存（上传完成后调用）
func (cl *CacheLayer) InvalidateFid(fid string)
```

### 设计决策

- **纯缓存，不碰加密**：`fetcher` 回调由 VFSImpl 定义，CacheLayer 只调不问。
- **L1 + L2 统一管理**：memCache miss 后自动查 diskCache，减少 fetcher 调用。
- **磁盘缓存是已有 `core/qrypt.CacheManager`**：CacheLayer 持其引用，putchunk 在 fetcher 回调中完成。
- **预取触发由 `NotifyRead` 驱动**：CacheLayer 内部维护 `SeqReadTracker`，检测到连续顺序读后自动预取后续块。

## 层四：UploadManager (internal/vfs/upload.go)

```go
package vfs

import (
    "context"
    "sync"
    "time"
    "github.com/yinzhenyu/skills/qrypt/drivers"
)

type UploadObserver interface {
    OnUploadComplete(path string, err error)
    OnUploadProgress(path string, bytes, total int64) // 可选
}

type UploadManager struct {
    executor  *OpExecutor        // 上传执行（调 CipherDriver.Put）
    observer  UploadObserver     // 回调 VFSImpl
    writeBack time.Duration      // writeBack 延迟窗口
    timers    map[string]*time.Timer
    retry     *RetryState        // 重试计数 + 退避策略
}

func NewUploadManager(executor *OpExecutor, writeBack time.Duration, observer UploadObserver) *UploadManager

// MarkDirty 标记文件为脏，启动 writeBack 定时器。
// 定时器到期前如有新的 MarkDirty，定时器重置。
func (um *UploadManager) MarkDirty(node *VFSNode)

// CancelSync 取消指定 path 的 pending 上传并停止定时器。
func (um *UploadManager) CancelSync(path string)

// IsPending 返回指定 path 是否有未完成的上传。
func (um *UploadManager) IsPending(path string) bool

// Enqueue 立即入队上传（绕过 writeBack 延迟，用于恢复后重试）。
func (um *UploadManager) Enqueue(node *VFSNode)
```

### 上传生命周期

```
VFS.Write()
  → staging 写入明文
  → um.MarkDirty(node)
    → 启动/重置 writeBack 定时器

定时器到期
  → um.onTimer(path, node)
    → executor.Upload(ctx, parentID, name, size, stagingReader)
      → CipherDriver.Put（内部加密）
    → 成功？
      ✓ → observer.OnComplete(path, nil)
            → VFSImpl 清理 tree 脏标记、删除 pending journal
      ✗ → retry.TryAgain()?
            ✓ → 退避后重入队
            ✗ → observer.OnComplete(path, err)
```

## 层五：OpExecutor (internal/vfs/executor.go)

```go
package vfs

import (
    "context"
    "io"
    "github.com/yinzhenyu/skills/qrypt/drivers"
)

// OpExecutor 是所有写操作的统一执行器。
// 不负责调度、重试、状态管理——只负责"执行一次操作"。
type OpExecutor struct {
    cipherDrv *CipherDriver
}

func NewOpExecutor(cipherDrv *CipherDriver) *OpExecutor

func (ex *OpExecutor) Upload(ctx context.Context, parentID, name string, size int64, body io.Reader) (drivers.Entry, error)
func (ex *OpExecutor) Remove(ctx context.Context, entry drivers.Entry) error
func (ex *OpExecutor) BatchRemove(ctx context.Context, entries []drivers.Entry) error
func (ex *OpExecutor) Rename(ctx context.Context, entry drivers.Entry, newName string) error
func (ex *OpExecutor) Mkdir(ctx context.Context, parentID, name string) (drivers.Entry, error)
```

**调用路径**：

```
VFSImpl.Remove()
  → tree.Delete(path)
  → coalescer.Submit(OpDelete{...})
    → flush → VFSImpl.handleCoalescedIntents()
      → executor.Remove/BatchRemove

UploadManager.onTimer()
  → executor.Upload()
```

## 层六：OperationCoalescer（迁入 internal/vfs/）

从 `internal/fusefs/` 迁入 `internal/vfs/`。其 `flushFn` 由 VFSImpl 的 `handleCoalescedIntents` 充当。内部逻辑完全不变。

## VFSImpl 完整定义

```go
// internal/vfs/vfsimpl.go
type VFSImpl struct {
    tree      *TreeManager         // 节点树操作（内聚，不独立出包）
    staging   *qrypt.Store         // 明文写缓存（core/qrypt/staging.go）
    cache     *CacheLayer          // 读缓存
    cipherDrv *CipherDriver        // 透明加密
    uploads   *UploadManager       // 上传调度
    coalescer *OperationCoalescer  // 操作合并
    executor  *OpExecutor          // 写操作执行
}

func NewVFSImpl(drv drivers.Driver, cp *cipher.RcloneCipher, cacheDir string, opts FSOptions) *VFSImpl {
    cipherDrv := cipherdriver.New(drv, cp)           // pkg/cipherdriver
    executor := NewOpExecutor(cipherDrv)
    staging, _ := qrypt.NewStore(filepath.Join(cacheDir, "staging"))
    cache := NewCacheLayer(opts.MemCacheSizeMB, diskCache, func(ctx, fid string, blockIdx uint64) ([]byte, error) {
        node := vfsImpl.tree.LookupByFid(fid)
        return cipherDrv.ReadBlock(ctx, nodeToEntry(node), blockIdx)
    })
    uploads := NewUploadManager(executor, opts.WriteBackTimeout, vfsImpl)
    coalescer := NewOperationCoalescer(caps, vfsImpl.handleCoalescedIntents)
    // ...
}
```

### VFSImpl 实现 VFS 接口的路径

| VFS 方法 | VFSImpl 内部流程 |
|---|---|
| `Stat` | `tree.Lookup(path)` → 返回 `NodeInfo` |
| `ListDir` | `tree.Lookup` → 过期则 `cipherDrv.List(fid)` → `MergeRemoteChanges` → 返回 children |
| `Read` | `staging.ReadAt`（有本地缓存）→ 否则 `cache.ReadBlock` 组装 |
| `Create` | `staging.Create` → `cp.GenerateRandomNonce` → `tree.Store` → `coalescer.Submit(OpCreate)` |
| `Write` | `staging.WriteAt` → `uploads.MarkDirty(node)` |
| `Mkdir` | `tree.Store` → `coalescer.Submit(OpMkdir)` |
| `Remove` | `uploads.CancelSync` → `tree.Delete` → `coalescer.Submit(OpDelete)` |
| `Rename` | `tree.ReplacePath` → `coalescer.Submit(OpRename)` |
| `Truncate` | staging 创建/下载 → `staging.Truncate` → 改 size → `uploads.MarkDirty` |
| `Flush` | staging flush → `uploads.MarkDirty(node)`（立即上传） |
| `Fsync` | 同 Flush |

## 迁移步骤

### Phase 1：准备（纯新增，不删代码）

1. 定义 `pkg/vfs/vfs.go` — VFS 接口
2. 定义 `pkg/cipherdriver/cipher_driver.go` — CipherDriver 实现
3. 创建 `internal/vfs/` 包骨架

### Phase 2：CipherDriver + 加密移动

1. 实现 `CipherDriver.ReadBlock/List/Mkdir/Remove/Rename/Put`
2. `EncryptingReader`、`DecryptingReader`、`EncryptAndPut` 从 `core/qrypt/` 移入 `cipher/`
3. 验证：`CipherDriver` 能通过 mock driver 的单元测试

### Phase 3：VFSImpl 实现

1. VFSNode + tree 操作从 `internal/fusefs/` 迁移到 `internal/vfs/`
2. CacheLayer 实现（L1 mem + L2 disk + dedup + prefetch）
3. UploadManager 实现（writeBack + 重试 + OpExecutor）
4. OpExecutor 实现
5. Coalescer 从 `internal/fusefs/` 迁入 `internal/vfs/`

### Phase 4：FUSE 适配层重写

1. 新 `internal/fusefs/fuse.go` 持 `pkg/vfs.VFS` 引用，纯协议转换
2. 逐一迁移 FUSE 回调，删除旧 `QryptFS` 方法
3. `internal/fusefs/` 只保留 fuse.go + 挂载代码 + coalescer（已迁出）

### Phase 5：清理

1. 删除 `internal/upload/`
2. 删除 `core/qrypt/` 中已移至 `cipher/` 的文件
3. `internal/fusefs/` 中删除旧 Node、旧 tree、旧 fs.go 的冗余逻辑
4. `MountManager` 改为构造 `internal/vfs.VFSImpl`
5. `go test ./internal/vfs/` 全平台通过

## VFSNode 定义

```go
// internal/vfs/node.go
type VFSNode struct {
    mu sync.RWMutex

    // 树结构
    fid         string
    parentFid   string
    name        string
    currentPath string
    isFolder    bool
    children    map[string]*VFSNode  // 仅目录有

    // 文件元数据
    size    int64
    encSize int64
    mtime   time.Time
}
```

不包含以下旧字段——它们各自移入对应组件：

| 旧 Node 字段 | 新归属 |
|---|---|
| `localPath` | staging（VFSImpl 直持 `qrypt.Store`） |
| `fileNonce` / `hasNonce` | CipherDriver 内部 nonceCache |
| `isDirty` / `syncQueued` | UploadManager 内部状态 |
| `uploading` / `uploadingChildren` | UploadManager |
| `lastReadBlock` / `readSeqCount` | CacheLayer.SeqReadTracker |
| `lastMetadataCheck` | VFSImpl.MergeRemoteChanges 逻辑 |
| `baseServerMtime` / `baseServerSize` | VFSImpl 上传完成回调 |
| `source` / `expectedFid` / `uploadedFid` | UploadManager / VFSImpl |
| `cancelled` / `oldRemoteFid` | UploadManager / OpExecutor |

## 迁移任务分解

### 并行依赖图

```
             同时开始
           ┌────┴────┐
           │         │
      Phase 1a   Phase 1b
      cipher/     VFS 接口
      迁移        定义（含 VFSNode）
           │         │
           └────┬────┘
                │
           Phase 2
        CipherDriver 实现
                │
           ┌────┴────┐
           │         │
      Phase 3a    Phase 3b
    CacheLayer   UploadMgr +
      + Tree     OpExecutor +
                  Coalescer
           │         │
           └────┬────┘
                │
           Phase 4
      FUSEAdapter + 切换
                │
           Phase 5
           清理旧代码
```

### Phase 1a：cipher/ 扩容

**改动文件：**
- `core/qrypt/encrypt_reader.go` → `cipher/encrypt_reader.go`（改 package）
- `core/qrypt/decrypt_reader.go` → `cipher/decrypt_reader.go`（改 package）
- `core/qrypt/encrypt_put.go` → `cipher/encrypt_put.go`（改 package，可选）

旧位置保留转发 import 或直接更新 `core/qrypt/` 内部引用为新路径。

**验证：** `go test ./cipher/` 通过。

### Phase 1b：VFS 接口 + VFSNode 定义

**新增文件：**
- `pkg/vfs/vfs.go`：VFS 接口、NodeInfo、DirEntry 类型
- `internal/vfs/node.go`：VFSNode struct（仅树 + 元数据）
- `internal/vfs/observer.go`：UploadObserver 接口

**验证：** 无测试（纯接口定义，编译通过即可）。

### Phase 2：CipherDriver

**新增文件：**
- `pkg/cipherdriver/cipher_driver.go`

**实现内容：**
- `ReadBlock(ctx, entry, blockIdx uint64)` — 从 `fetchBatch` 提取加密相关逻辑（nonce 提取、偏移换算、DecryptBlock）
- `List(ctx, parentID)` — 解密文件名
- `Mkdir(ctx, parentID, name)` — 加密文件名
- `Remove(ctx, entry)` — 透传
- `Rename(ctx, entry, newName)` — 加密新名
- `Put(ctx, parentID, name, size, body)` — 内部 nonce + encryptContent
- `Read` 接口透传密文

**验证：** mock driver + real cipher → `go test ./pkg/cipherdriver/` 覆盖 ReadBlock（正常/边界/401 重试）、List、Mkdir、Put。

### Phase 3a：CacheLayer + Tree（与 Phase 3b 并行）

**新增文件：**
- `internal/vfs/tree.go` — 树操作（Store、Lookup、ReplacePath、DeletePath）
- `internal/vfs/cache.go` — CacheLayer（memCache + diskCache + dedup + prefetch）
- `internal/vfs/vfsimpl.go` — VFSImpl 骨架（构造 + 子管理器持有）

**验证：** VFSImpl 可被构造，tree 操作通过单元测试。

### Phase 3b：UploadManager + OpExecutor + Coalescer（与 Phase 3a 并行）

**新增/迁入文件：**
- `internal/vfs/executor.go` — OpExecutor（调 CipherDriver）
- `internal/vfs/upload.go` — UploadManager（writeBack + 重试）
- `internal/vfs/coalescer.go` — 从 `internal/fusefs/` 迁入（代码不变）

**VFSImpl 实现各 VFS 方法：**
- Stat / ListDir / Read / Create / Write / Mkdir / Remove / Rename / Truncate / Utimens / Flush / Fsync
- MergeRemoteChanges 逻辑移植
- 恢复逻辑（replayOpsLog / recoverDirtyFiles）

**验证：** mock CipherDriver + mock driver → `go test ./internal/vfs/` 覆盖全部 VFS 方法。

### Phase 4：FUSEAdapter + 切换

**新增文件：**
- `internal/fusefs/fuse.go` — 新 FUSEAdapter，持 `vfs.VFS` 引用
  - Getattr / Access / Readdir / Read / Write / Create / Mkdir / Unlink / Rmdir / Rename / Truncate / Flush / Fsync / Statfs
  - Open / Release / Releasedir / Chmod / Chown / xattr → 无操作或透传

**修改文件：**
- `internal/fusefs/mount_fuse.go` — `mount()` 中构造 VFSImpl 替代 NewFS

**验证：** e2e 测试 —— 挂载后 ls / cat / write / rm / mv / truncate 结果与旧版一致。

### Phase 5：清理

**删除文件：**
- `internal/upload/` 整包
- `core/qrypt/encrypt_reader.go`、`decrypt_reader.go`、`encrypt_put.go`（已移至 cipher/）
- `internal/fusefs/` 中旧文件：fs.go、getattr.go、readdir.go、read.go、write.go、delete.go、rename.go、mkdir.go、sync.go、workers.go、tree.go、node.go、recovery.go（部分可复用代码先迁移后删）

**修改文件：**
- `internal/mount/manager.go` — startLocked 中构造 VFSImpl 替代 QryptFS
- `internal/fusefs/fs_nofuse.go` — 对应更新 stub

**最终验证：**
- `go test ./internal/vfs/` 全平台通过
- `go test ./internal/fusefs/`（无 FUSE C 库时跳过）
- `go build ./cmd/qrypt` 成功

## 不变的部分

- `OperationCoalescer`：逻辑完全不变，仅从 `internal/fusefs/` 迁至 `internal/vfs/`
- `core/qrypt.CacheManager`：供 CacheLayer L2 diskCache 使用，接口不变
- `core/qrypt.Store`（staging）：VFSImpl 直持，接口不变
- `drivers.Driver/Writer/Uploader` 接口：CipherDriver 实现它们，接口本身不变
- `MountManager`：构造 VFSImpl 替代 QryptFS，对外接口不变
