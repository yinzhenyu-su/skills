## Context

qrypt 当前架构是从单进程 FUSE 挂载工具逐步生长出来的：

```
v1: qrypt mount (单进程, 1 driver + 1 FUSE)
v2: + qryptd (daemon skeleton, 基本管理 RPC)
v3: + multi-mount (多个 mount 实例)
v4: + daemon push/pull (CLI 通过 RPC 路由文件操作)
```

当前设计将 daemon 视为"可选加速层"，CLI 保留全部能力。这带来了 IPC 协议割裂（JSON-RPC 无原生推送和数据帧）、资源浪费（重复创建 Driver）、以及两层上传系统并行的问题。

本设计重新以 daemon 为中心，将 CLI 降级为面向 daemon 的薄客户端。

## Goals / Non-Goals

**Goals:**
- 单连接双工 IPC，同时处理 RPC、事件推送、数据流
- Driver 连接池化，CLI 和 VFS 复用同一认证会话
- 统一上传调度，合并 VFS flushing 和 CLI push
- CLI 移除所有直接业务逻辑，委托 daemon 执行
- 缓存一致性事件机制

**Non-Goals:**
- 跨语言/跨平台协议（WebSocket 已足够）
- 客户端与服务端分离部署（始终同一台机器）
- 水平扩展（单用户场景无用）

## Architecture

```
┌────────────────────────────────────────────────────────────────────┐
│ qryptd                                                                 │
│                                                                         │
│  ┌──────────────────────────────────────────┐                          │
│  │ WebSocket Server (Unix socket)            │                          │
│  │  ~/.qrypt/qryptd.sock                     │                          │
│  │                                           │                          │
│  │  Text Frame   → JSON: req/resp/event      │                          │
│  │  Binary Frame → 文件内容数据                    │                          │
│  │  单连接全双工                                  │                          │
│  └──────────────────────────────────────────┘                          │
│                                                                         │
│  ┌──────────────────────────────────────────┐                          │
│  │ SessionManager                            │                          │
│  │                                           │                          │
│  │  每个驱动类型/凭证组合一个 Session               │                          │
│  │  引用计数, 共享给 VFS + CLI                   │                          │
│  │                                           │                          │
│  │  ┌─────────────────────────────────┐      │                          │
│  │  │ Session "quark.cookieA"         │      │                          │
│  │  │  ├─ Drv: *quark.Driver          │      │                          │
│  │  │  ├─ RefCount: 2 (VFS + borrow)  │      │                          │
│  │  │  └─ TokenRefresher: goroutine   │      │                          │
│  │  └─────────────────────────────────┘      │                          │
│  │  ┌─────────────────────────────────┐      │                          │
│  │  │ Session "yun139.cookieB"        │      │                          │
│  │  │  ├─ Drv: *yun139.Driver         │      │                          │
│  │  │  ├─ RefCount: 1 (VFS)           │      │                          │
│  │  │  └─ TokenRefresher: goroutine   │      │                          │
│  │  └─────────────────────────────────┘      │                          │
│  └──────────────────────────────────────────┘                          │
│                                                                         │
│  ┌──────────────────────────────────────────┐                          │
│  │ VirtualFS                                     │                          │
│  │                                               │                          │
│  │  Mount "personal" (FUSE: ~/QuarkPersonal)     │                          │
│  │  ├─ 目录树（内存 LRU）                           │                          │
│  │  ├─ 块缓存（内存 LRU）                              │                          │
│  │  ├─ Staging（磁盘暂存）                               │                          │
│  │  └─ Session 引用（borrow from SessionManager）         │                          │
│  │                                               │                          │
│  │  Mount "work" (FUSE: ~/QuarkWork)              │                          │
│  │  └─ 同上                                   │                          │
│  └──────────────────────────────────────────┘                          │
│                                                                         │
│  ┌──────────────────────────────────────────┐                          │
│  │ TransferOrchestrator                      │                          │
│  │                                           │                          │
│  │  统一队列:                                    │                          │
│  │    [VFS flush: doc.docx]                  │                          │
│  │    [CLI push: video.mp4]                  │                          │
│  │    [VFS flush: photo.jpg]                 │                          │
│  │                                           │                          │
│  │  TokenBucket: 全局上传限速                       │                          │
│  │  WorkerPool: 并发控制                          │                          │
│  │  ProgressHub: 统一进度推送                        │                          │
│  │  MultipartManager: 断点续传管理                  │                          │
│  └──────────────────────────────────────────┘                          │
│                                                                         │
│  ┌──────────────────────────────────────────┐                          │
│  │ CacheInvalidator                          │                          │
│  │                                           │                          │
│  │  监听传输完成事件 → 通知对应 VFS 驱逐缓存        │                          │
│  └──────────────────────────────────────────┘                          │
└────────────────────────────────────────────────────────────────────┘

┌──────────────────────────┐     ┌─────────────────────────────┐
│ qrypt CLI                │     │ macOS Finder / cp / ls       │
│                          │     │                              │
│ 薄客户端, 仅做:            │     │ FUSE syscall                 │
│  ├─ 拼装 JSON request     │     │  → QryptFS (daemon 进程内)    │
│  ├─ 收 JSON response      │     │  → borrow Session            │
│  ├─ 中继 Binary 数据流      │     │  → 返回结果或 staging+enqueue │
│  └─ 无 config/driver/cipher│     │                              │
└──────────────────────────┘     └─────────────────────────────┘
```

## Decisions

### Decision 1: WebSocket over Unix socket

**方案：** 使用 `nhooyr.io/websocket` 在 Unix socket 上运行 WebSocket 协议。

**理由：**
- 原生支持 Text/Binary 帧，自然区分 JSON 控制消息和二进制数据
- 单连接全双工，RPC 请求和事件推送共享同一连接
- 内置 ping/pong 心跳、关闭握手、消息分帧
- 若未来需要远程管理（TCP），Socket 层替换为 `net.Dial("tcp", ...)` 即可，上层协议不变

**不选择 gRPC：** protobuf schema 编译、service 定义、HTTP/2 栈的复杂度对单机工具来说过重。

**不选择自定义帧协议：** WebSocket 提供的帧边界、心跳、关闭握手正是需要手写且容易出 bug 的部分。一个成熟库比自研更可靠。

**协议格式：**

```
Text Frame → JSON 消息:

请求:  {"id":1, "method":"ls",     "params":{"path":"/"}}
响应:  {"id":1, "result":{"entries":[...]}}
事件:  {"id":0, "method":"event",  "params":{"type":"progress","data":{...}}}
错误:  {"id":1, "error":{"code":-1, "message":"not found"}}

Binary Frame → 文件数据:

Push:  CLI → daemon (body: 加密文件分片)
Pull:  daemon → CLI (body: 解密文件分片)
Cat:   daemon → CLI (body: 解密后内容流)
```

### Decision 2: SessionManager — 共享池（方案 A）

**方案：** 所有消费者（VFS、CLI RPC）从同一个 Session 池 borrow/release。

```go
type SessionKey struct {
    Type    string   // "quark", "yun139", "localfs"
    CredKey string   // 凭证哈希（cookie hash, access key hash）
}

type Session struct {
    Drv      drive.Driver
    RefCount atomic.Int32
    Token    atomic.Value  // 存储最新 token, 后台 goroutine 刷新
    closeCh  chan struct{} // 通知刷 token 的 goroutine 退出
}

type SessionManager struct {
    mu       sync.Mutex
    sessions map[SessionKey]*Session
}

func (sm *SessionManager) Acquire(key SessionKey) (*Session, error)
func (sm *SessionManager) Release(key SessionKey)
```

**流程：**
- `Acquire()`: 检查 `sessions[key]` 是否存在，存在则 `RefCount++`，不存在则新建 Driver → `Init()` → 启动 token refresh goroutine → `RefCount=1`
- `Release()`: `RefCount--`，到 0 时关闭 token refresh goroutine → `Driver.Drop()` → 从 map 删除

**锁竞争分析：**
- Acquire/Release 只在 `mu.Lock()` 内做 map lookup + refcount 原子增减
- 临界区极短（纳秒级）
- 实际的 `drv.List()` / `drv.Read()` 调用在 Release 之后、Driver 方法调用不在锁保护范围内
- VFS 的 Getattr 如果缓存命中，根本不碰 SessionManager
- 真实场景下锁竞争不可感知

**不选择方案 B（VFS 独享 session）：**
- 每个 VFS mount 多一个 TCP 连接和认证握手
- 如果 3 个 mount 连同一个夸克账号，3 个独立 TCP 连接 vs 1 个共享连接
- 没有实质好处（"锁竞争"在实测中是理论问题）

**Token 刷新：**
- Quark Driver 的 token（`__puus`）在 HTTP 请求中自动更新
- 用 `atomic.Value` 存储最新 token，后台 goroutine 定期获取或被动接收更新
- 不在锁保护范围内做 token 刷新，避免 VFS 请求被 token 刷新阻塞

### Decision 3: 统一上传调度（TransferOrchestrator）

**方案：** 单队列吞掉 VFS flushing 和 CLI push。

```go
type UploadTask struct {
    Type      TaskType      // FlushFile, PushFile
    Mount     string        // "personal", "work"
    RemotePath string
    LocalPath  string       // staging 文件路径（FlushFile）或系统文件路径（PushFile）
    Size      int64
    Callback  func(error)   // 完成后通知调用者
}

type TransferOrchestrator struct {
    queue      chan UploadTask
    workers    []*transferWorker
    tokenBucket *rate.Limiter
    progressHub *ProgressHub
}
```

**UploadTask 流转：**

```
VFS Release()                     CLI qrypt push file
       │                                │
       ▼                                ▼
  orchestrator.Enqueue(task)     orchestrator.Enqueue(task)
       │                                │
       └────────────┬───────────────────┘
                    ▼
            queue (cap=2000)
                    │
                    ▼
            worker goroutine × N
                    │
                    ├─ uploader.Put(ctx, body)
                    ├─ progressHub.Publish(progress)
                    ├─ tokenBucket.Wait(ctx, fileSize)
                    └─ callback(err)
```

**VFS side：**
- Release() 行为不变：`staging.Sync()` → `orchestrator.Enqueue()` → 返回
- 原来 Release 后马上返回，现在也一样

**v.s. 当前架构：**

| | 当前 | 统一后 |
|--|------|--------|
| VFS flush 队列 | uploadChan (cap 1000) | TransferOrchestrator.queue |
| CLI push 队列 | WorkerPool.jobs | TransferOrchestrator.queue（同一队列） |
| 并发控制 | VFS: sync.concurrency / CLI: --transfers | 统一并发数 |
| 限速 | 无 | Token bucket |
| 进度查询 | CLI push 有 event, VFS flush 无 | 统一 ProgressHub, status 可见 |
| 重试策略 | 各写各的 | 统一重试逻辑 |

值得注意：当前 Release 已经是异步 enqueue 的，所以这个变更**对 VFS 行为无影响**。用户无感知。

### Decision 4: 薄 CLI

**CLI 不再直接做的事情：**
- 加载和解析 TOML config（→ 由 daemon 持有）
- 创建 cipher 实例（→ daemon 持有）
- 创建 Driver 实例（→ daemon SessionManager）
- 加密/解密文件内容（→ daemon cipher）
- 上传/下载文件（→ daemon TransferOrchestrator）
- 遍历本地目录树、构建传输清单（→ daemon 收到路径后自己 Scan）

**CLI 保留的能力：**
- `qrypt init` → 本地生成配置模板（不依赖 daemon）
- `qrypt validate` → 本地校验配置（不依赖 daemon）
- `qrypt config` → 查看/修改配置（委托 daemon）
- 其余全部命令 → WebSocket → daemon

**命令映射表：**

| 命令 | 当前 | 理想 |
|------|------|------|
| `qrypt mount` | 直接启 FUSE | WS → daemon mount start |
| `qrypt ls` | 混合（daemon/direct） | WS → daemon |
| `qrypt push` | 混合（daemon/direct） | WS → daemon（binary data 流） |
| `qrypt pull` | 混合（daemon/direct） | WS → daemon（binary data 流） |
| `qrypt cat` | direct | WS → daemon（binary data 流） |
| `qrypt find` | direct | WS → daemon（daemon 返回全量再在 CLI filter）|
| `qrypt cp` | direct | WS → daemon（跨盘传输走 daemon 内流式管道）|
| `qrypt rm` | 混合（daemon/direct） | WS → daemon |
| `qrypt mv` | 混合（daemon/direct） | WS → daemon |
| `qrypt status` | 混合 | WS → daemon |
| `qrypt init` | local | local（不变） |
| `qrypt validate` | local | local（不变） |

**需要考虑：** `qrypt cat` 走 daemon 意味着即使小文件也需要一次 IPC 往返。但 WebSocket binary frame 传递文件内容的开销极小（mmap 共享内存实际上更复杂），对于个人工具可以接受。

### Decision 5: FUSE 嵌入 Daemon 进程

**方案：** `qryptd` 是唯一能启 FUSE 挂载的进程。`qrypt mount` 命令变为向 daemon 发送 `mount_start` RPC 的别名。

**理由：**
- 所有 VFS 操作直接访问 daemon 内的 SessionManager 和缓存
- daemon 退出时自动卸载所有挂载点（通过 signal handler）
- 消除"独立 mount 进程"和"daemon mount"两条路径的维护成本

**FUSE 降级场景：** 如果 daemon 没有运行，`qrypt mount` 报错提示启动 daemon。这与当前"daemon 不可用则 fallback 到 direct"的策略不同——是明确的设计取舍。

### Decision 6: CacheInvalidator

**方案：** 一个轻量的事件监听器，在传输完成后通知对应 VFS 驱逐缓存。

```go
type CacheInvalidator struct {
    vfsMap map[string]*QryptFS  // mount name → VFS
}

// 当 TransferOrchestrator 完成一个文件上传后:
func (ci *CacheInvalidator) OnFileUploaded(mount, remotePath string) {
    if vfs, ok := ci.vfsMap[mount]; ok {
        vfs.InvalidateCache(remotePath)  // 驱逐目录树和块缓存
    }
}

// 当 CLI pull 或 daemon 操作完成后:
func (ci *CacheInvalidator) OnFileRemoved(mount, remotePath string) {
    if vfs, ok := ci.vfsMap[mount]; ok {
        vfs.EvictEntry(remotePath)  // 从目录树删除 + 块缓存驱逐
    }
}
```

**为什么需要：**
- 当前 VFS 对"外部"变更毫无感知
- CLI push 了一个文件 → VFS 目录树缓存还是旧的 → `ls` 看不到新文件
- 另一个设备删了文件 → VFS 缓存等到 TTL 过期才反映

**实现注意：** 缓存驱逐是轻量操作（从 LRU map 删除条目），不会阻塞传输完成路径。

## Data Flows

### 正常读文件（通过 FUSE）

```
Finder read /report.pdf
  │
  ▼
FUSE 内核 → QryptFS.Read(path, offset, size)
  │
  ├─ 检查块缓存 (LRU Cache) → 命中则直接返回
  │
  ├─ 未命中:
  │    ├─ session := sm.Acquire("quark")    // refcount++
  │    ├─ drv.Read(ctx, entry, offset, size)
  │    ├─ cipher.Decrypt(chunk)             // 解密
  │    ├─ cache.Put(chunk)                  // 写入缓存
  │    └─ sm.Release("quark")               // refcount--
  │
  └─ 返回明文给 FUSE 内核
```

### 写文件并同步（通过 FUSE）

```
Finder copy video.mp4 → FUSE 挂载点
  │
  ▼
QryptFS.Write(path, data, offset)
  │
  ├─ staging.WriteAt(offset, data)          // 写入暂存区
  └─ node.dirty = true
  │
  ▼ (稍后, Finder close 文件)
QryptFS.Release(path)
  │
  ├─ staging.Sync(localPath)               // 刷到磁盘文件
  └─ orchestrator.Enqueue({
         Type: FlushFile,
         Mount: "personal",
         RemotePath: "/video.mp4",
         LocalPath: "/tmp/staging/xyz",
     })
  │
  ▼ (后台, TransferOrchestrator worker)
worker goroutine
  ├─ tokenBucket.Wait(fileSize)
  ├─ uploader.Put(ctx, parentID, encName, size, body)
  ├─ progressHub.Publish(ProgressInfo{...})
  └─ cacheInvalidator.OnFileUploaded("personal", "/video.mp4")
```

### CLI push 文件

```
qrypt push ~/video.mp4 /remote/video.mp4
  │
  ▼
CLI → WS Text: {"id":1, "method":"push",
                 "params":{"source":"~/video.mp4","remote":"/remote/video.mp4"}}
  │
  ▼
Daemon → WS Binary: [file body in encrypted chunks]
  │
  ▼
Daemon → orchestrator.Enqueue(...)
  │
  ▼
Daemon → WS Text: {"id":1, "result":{"task_id":"..."}}
  │
  ▼ (后台)
worker → uploader.Put → progressHub → WS Text: {"id":0, "method":"event",
                                                    "params":{"type":"progress",...}}
```

### CLI cat 文件

```
qrypt cat /remote/file.txt
  │
  ▼
CLI → WS Text: {"id":1, "method":"cat",
                 "params":{"path":"/remote/file.txt"}}
  │
  ▼
Daemon → borrow Session → drv.Read → cipher.Decrypt
  │
  ▼
Daemon → WS Binary: [decrypted text content chunk]
Daemon → WS Binary: [more content]
Daemon → WS Text: {"id":1, "result":{"eof":true}}
```

**注意：** cat 走 daemon 比当前直接实现多一次 IPC。但 WebSocket binary 帧的开销 ≈ 零拷贝（Go 的 `WriteMessage` 直接发 `[]byte`），实测差异可忽略。如果以后有超高频 cat 需求，可加 `--direct` 标志走原路径——但当前无此必要。

## Session Key 的"凭证哈希"如何算

**问题：** 同一个夸克账号的两个 mount（不同的 root_path），应该复用同一个 Session 还是分开？

**决策：** **复用。** Session 是驱动层连接，跟 mount 路径无关。

```go
type SessionKey struct {
    Type    string // "quark"
    CredKey string // sha256(cookie) 截取前 16 字节
}
```

两个 mount 如果 cookie 相同 → 同一 Session。不同的 root_path 在 `drv.ResolvePath()` 时传入。

如果用户有两个夸克账号（不同 cookie）→ 两个 Session，各自独立连接。

## Tasks

Task 列表以"从当前代码演进到目标架构"的视角编写，分为 4 个阶段。

### Phase 1: IPC 层替换（重构, 不影响业务逻辑）

- [ ] 1.1 添加 `nhooyr.io/websocket` 依赖
- [ ] 1.2 实现 `internal/daemon/wsserver.go` — WebSocket Server（接受连接, 分帧, dispatch）
- [ ] 1.3 实现 `internal/daemon/wsclient.go` — WebSocket Client（请求/响应, 事件接收, binary 收发）
- [ ] 1.4 旧 JSON-RPC Server 包装为 WebSocket Text frame handler（兼容桥接, 逐步迁移）
- [ ] 1.5 `SubscribeEvents` 退役 — 事件改由同一连接推送
- [ ] 1.6 测试：WebSocket 握手、并发请求、二进制帧、事件推送

### Phase 2: SessionManager + 薄 CLI（基础设施, 独立验证）

- [ ] 2.1 实现 `internal/daemon/session.go` — SessionManager + Session 核心
- [ ] 2.2 实现后台 TokenRefresher goroutine（每种驱动类型的刷新逻辑）
- [ ] 2.3 VFS 改为从 SessionManager borrow session（移除 VFS 持久的 Driver 实例）
- [ ] 2.4 `withTempMount` 退役 — 所有 daemon RPC handler 改用 SessionManager
- [ ] 2.5 CLI 移除 config/cipher/driver 创建逻辑（仅 init/validate 保留本地操作）
- [ ] 2.6 `loadToolCfg` / `loadToolCfgOnly` / `loadToolDriverForMount` → 删除或简化为 WS RPC
- [ ] 2.7 测试：session 复用、refcount 正确性、token 刷新、CLI 无 config 场景

### Phase 3: TransferOrchestrator（行为变化, 需要仔细回归）

- [ ] 3.1 实现 `internal/daemon/orchestrator.go` — TransferOrchestrator
- [ ] 3.2 VFS staging flush 由 enqueue to uploadChan → enqueue to orchestrator
- [ ] 3.3 CLI push 由 WorkerPool → orchestrator
- [ ] 3.4 实现 TokenBucket 全局限速
- [ ] 3.5 实现 ProgressHub（统一进度推送, 替代当前两套 event 路径）
- [ ] 3.6 `internal/sync/pool.go` WorkerPool 退役（但 Downloader 和 Scan 逻辑保留）
- [ ] 3.7 测试：上传并发、限速、VFS+CLI 混合场景、进度推送

### Phase 4: 缓存一致性 + FUSE 归入 daemon（收尾）

- [ ] 4.1 实现 CacheInvalidator — 监听传输完成事件, 通知 VFS 驱逐缓存
- [ ] 4.2 `qrypt mount` 改为 WS → daemon mount start 别名（移除独立 mount 进程路径）
- [ ] 4.3 cat/find/cp 改为走 daemon（binary frame 数据流）
- [ ] 4.4 回归测试：所有 CLI 命令、FUSE 操作 无行为变化
- [ ] 4.5 清理：删除废弃的 JSON-RPC 代码、direct 代码路径

## Risks / Trade-offs

| Risk | Mitigation |
|------|-----------|
| WebSocket 依赖不是标准库 | `nhooyr.io/websocket` 纯 Go、广泛使用、LICENSE MIT；依赖数量增加 1 个 |
| Daemon 进程崩溃 → CLI 全部瘫痪 | daemon 退出自动卸载 FUSE, 重启后 VFS 恢复正常；CLI 显示友好错误"daemon 未运行" |
| Session 共享 → VFS 被 CLI 操作影响性能 | 锁竞争窗口纳秒级；若实测有问题, 可回退到方案 B（VFS 独享 session） |
| TransferOrchestrator 单点故障 | goroutine panic 只影响单个 worker, 不崩 Orchestrator；队列持久化暂不考虑 |
| CLI 变薄后 `qrypt cat` 延迟增加 | WebSocket 二进制帧开销极小；如有实际性能问题, 可给 cat 加 `--direct` 绕过 daemon |
| 迁移过程中旧代码需要保留 | Phase 1-4 逐阶段演进, 每阶段可独立部署；旧 JSON-RPC 路径在 Phase 4 才移除 |
