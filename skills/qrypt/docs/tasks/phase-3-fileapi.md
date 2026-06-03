# Phase 3: 实现 FileAPI (1.5d)

目标：实现 `core/qrypt.FileAPI`，作为跨平台文件操作的统一入口。

前置依赖：Phase 1（接口定义）+ Phase 2（Core Services）。

## 任务 3.1: FileAPI 结构体 + NewFileAPI 构造函数

### 参考

- 架构文档 D1 (L194-230)、D8 (L471-501)
- `daemon/service.go:22-36` 中 Daemon 结构体的组件组成

### 定义

```go
// core/qrypt/api.go

type Options struct {
    Driver       Driver
    Cipher       Cipher     // 对称加密引擎（*crypt.RcloneCipher 的包装）
    Dirs         DirResolver
    Creds        CredentialStore
    NetCfg       NetworkConfig
    BgTask       BackgroundTask
    Lifecycle    AppLifecycle
    RateLimitBPS int64       // bytes per second, 0 = unlimited
    NumWorkers   int         // upload workers, default 3
}

type FileAPI struct {
    drv      Driver
    cp       Cipher
    dirs     DirResolver
    creds    CredentialStore
    netCfg   NetworkConfig

    sessions  *SessionManager
    uploadQ   *Orchestrator
    events    *EventManager
    progress  *ProgressHub
    cacheInv  *CacheInvalidator
    rateLimit *RateLimiter
}

func NewFileAPI(ctx context.Context, opts Options) (*FileAPI, error) {
    // 1. 创建 SessionManager
    // 2. 创建 EventManager
    // 3. 创建 ProgressHub(em)
    // 4. 创建 RateLimiter
    // 5. 创建 Orchestrator(nWorkers, rl, ph)
    // 6. 创建 CacheInvalidator(hooks, em) — hooks 从 opts 或 nil
}
```

### Cipher 接口定义

`internal/crypt.RcloneCipher` 是具体的加解密实现，FileAPI 不直接引用。在 `core/qrypt/` 中定义：

```go
type Cipher interface {
    EncryptSegment(plain string) string
    DecryptSegment(cipher string) (string, error)
    EncryptedSize(plainSize int64) (int64, error)
    DecryptedSize(cipherSize int64) (int64, error)
}
```

适配器在桌面 CLI 端实现：`internal/crypt.RcloneCipher` → `core/qrypt.Cipher`。

## 任务 3.2: 实现 List / Stat / Find

### List

参考 `daemon/service.go:632-682`。

```go
func (a *FileAPI) List(ctx context.Context, mount, path string) ([]FileEntry, error)
```

逻辑：
1. `a.sessions.Acquire` → 获取 driver
2. `resolver.ResolvePath(path)` → fid
3. `drv.List(ctx, fid)` → 条目
4. `cipher.DecryptSegment` → 解密文件名（失败时保留加密名）
5. 返回 `[]FileEntry`

### Stat

```go
func (a *FileAPI) Stat(ctx context.Context, mount, path string) (*FileEntry, error)
```

逻辑：
1. 解析路径 → parentFid + baseName
2. List parent → 查找匹配项
3. 如果 `path` 是目录本身：`drv.List` → 取父目录的统计信息（跨目录时特殊处理）
4. 返回单个 FileEntry

### Find

参考 `daemon/service.go:685-801`。

```go
func (a *FileAPI) Find(ctx context.Context, mount, path, pattern string, maxDepth, maxMatches int, caseSensitive bool) ([]FileEntry, error)
```

递归遍历 `walkEntries` + 文件名匹配。

## 任务 3.3: 实现 Mkdir / Move / Remove

### Mkdir

参考 `daemon/service.go:804-862`，以及 `createRemoteDir` (1217-1251)。

```go
func (a *FileAPI) Mkdir(ctx context.Context, mount, path string) error
```

逻辑：
1. 检查 `mount` 是否存在（session 获取）
2. 解析 `path` → parentPath + dirName
3. 检查已存在
4. 加密目录名 → `writer.Mkdir`

支持 `-p` 语义（递归创建）作为二阶段。

### Move

参考 `daemon/service.go:940-1051`。

```go
func (a *FileAPI) Move(ctx context.Context, mount, oldPath, newPath string) error
```

逻辑：
1. 解析源/目标路径
2. Move（跨父目录）+ Rename（同目录改名）
3. no-clobber 检查

**注意**：`service.go:1042-1048` 中的 FUSE 节点树更新是桌面特有的，core/ 中不做。由 daemon/ 监听 Move 完成事件后自行处理。

### Remove

参考 `daemon/service.go:865-937`。

```go
func (a *FileAPI) Remove(ctx context.Context, mount, path string, recursive, force bool) error
```

逻辑：
1. 路径解析 + 查找目标
2. 检查空目录（非 recursive 时）
3. `writer.Remove`

## 任务 3.4: 实现 Push / Pull

### Push

参考 `daemon/service.go:315-541`。

```go
func (a *FileAPI) Push(ctx context.Context, mount, localPath, remotePath string, opts PushOptions) (taskID string, err error)
```

逻辑：
1. 打开本地文件
2. Acquire session
3. 解析远程路径 → parentFid
4. 使用 `syncpkg.Uploader` 上传（通过 `a.uploadQ.Submit`）
5. 发布进度事件

**架构文档 D1 说 Push/Pull 是文件级操作**。目录级递归（push directory）在 FileAPI 层面实现。

### Pull

参考 `daemon/service.go:1055-1215`。

```go
func (a *FileAPI) Pull(ctx context.Context, mount, remotePath, localPath string, opts PullOptions) (taskID string, err error)
```

逻辑对称于 Push。

## 任务 3.5: 实现 Read / Write（文件级）

### Read

```go
func (a *FileAPI) Read(ctx context.Context, mount, path string) (io.ReadCloser, error)
```

适合桌面 CLI `cat`。文件级读取：
1. 解析路径 → 获取 entry
2. `drv.Read(ctx, entry, 0, size)` → 原始 Reader
3. 包装解密 Reader（`crypt.DecryptReader`）

**不要**做缓冲或分段：FUSE 场景的 block 级随机访问不走 FileAPI。

### Write

文件级写入（适合 `push` 语义），不是 FUSE 的 block 级写入。

参考架构文档 D1 (L211-212)：
> Write 写入文件内容（暂存到 staging，异步加密上传）

## 任务 3.6: MobileAPI (gomobile 友好包装)

参考架构文档 D9 (L511-560)。

```go
// core/qrypt/mobile.go  (build tag: gomobile)

type MobileAPI struct {
    inner *FileAPI
}

func (m *MobileAPI) List(ctx, mount, path string) (string, error)  // JSON
func (m *MobileAPI) ReadData(ctx, mount, path string) ([]byte, error)
func (m *MobileAPI) WriteData(ctx, mount, path string, data []byte) error
func (m *MobileAPI) PushFile(ctx, mount, localPath, remotePath string) (string, error)
func (m *MobileAPI) PullFile(ctx, mount, remotePath, localPath string) (string, error)
func (m *MobileAPI) StatusJSON() string
```

gomobile 限制：不能有 `io.Reader`、`interface{}`、`chan`、`func` 参数。

## 验证方式

```bash
cd skills/qrypt && go build ./core/qrypt/
cd skills/qrypt && go vet ./core/qrypt/
cd skills/qrypt && go test ./core/qrypt/       # 全部 mock
cd skills/qrypt && go build ./cmd/qrypt/       # daemon 适配后编译
```
