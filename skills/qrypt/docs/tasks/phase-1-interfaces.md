# Phase 1: 定义 core/qrypt/ 接口 (0.5d)

目标：在 `core/qrypt/` 中定义所有接口，不涉及实现。

## 任务 1.1: core/qrypt/drive.go — 驱动消费者接口

参考 `docs/core-architecture.md` D12 (L643-663) 和 `internal/drive/driver.go`。

### Driver 接口（精简版）

```go
package qrypt

type Entry struct {
    ID      string
    Name    string
    IsDir   bool
    Size    int64
    ModTime time.Time
}

type Driver interface {
    Init(ctx context.Context) error
    Drop(ctx context.Context) error
    List(ctx context.Context, parentID string) ([]Entry, error)
    Read(ctx context.Context, entry Entry, offset, size int64) (io.ReadCloser, error)
}
```

与 `internal/drive.Driver` 的差异：
- 去掉了 `ParentID`、`Extra` 字段
- 没有 `drive.Meta` / `drive.Reader` 的子接口拆分

### Writer 接口

```go
type Writer interface {
    Mkdir(ctx context.Context, parentID, name string) (Entry, error)
    Move(ctx context.Context, entry Entry, dstParentID string) error
    Rename(ctx context.Context, entry Entry, newName string) error
    Remove(ctx context.Context, entry Entry) error
}
```

### Uploader 接口

```go
type Uploader interface {
    Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (Entry, error)
}
```

### PathResolver 接口

```go
type PathResolver interface {
    ResolvePath(ctx context.Context, path string) (string, error)
}
```

### 注意事项
- **不要**复制 `internal/drive` 的包路径，这是新定义
- 实现适配在 Phase 3-5 由各自调用方完成类型断言或包装
- 现有 quark.QuarkDriver 实现了 `internal/drive.Driver` 但不直接实现 `core/qrypt.Driver`——需要通过适配器或 duck typing 桥接

## 任务 1.2: core/qrypt/platform.go — Platform Services 接口族

参考 `docs/core-architecture.md` D7 (L399-467)。

```go
package qrypt

// DirResolver 解析各平台的应用数据目录
type DirResolver interface {
    CacheDir() string
    DataDir() string
    ConfigDir() string
}

// CredentialStore 安全凭据存取
type CredentialStore interface {
    Get(key string) (string, error)
    Set(key, value string) error
    Delete(key string) error
}

// NetworkConfig 网络配置
type NetworkConfig interface {
    HTTPClient() *http.Client
}

// BackgroundTask 后台任务调度抽象
type BackgroundTask interface {
    RunAsync(ctx context.Context, id string, fn func(ctx context.Context) error) error
    OnCompleted(id string) <-chan error
}

// AppLifecycle 生命周期通知
type AppLifecycle interface {
    Suspended() <-chan struct{}
    Resumed() <-chan struct{}
}
```

### 默认空实现（Noop 版本）

桌面 CLI 不需要 BackgroundTask 和 AppLifecycle 的完整实现，提供 noop：

```go
type NoopBackgroundTask struct{}
func (NoopBackgroundTask) RunAsync(ctx context.Context, id string, fn func(ctx context.Context) error) error {
    go fn(ctx)
    return nil
}
func (NoopBackgroundTask) OnCompleted(id string) <-chan error {
    ch := make(chan error, 1)
    return ch
}

type NoopAppLifecycle struct{}
func (NoopAppLifecycle) Suspended() <-chan struct{} {
    return nil
}
func (NoopAppLifecycle) Resumed() <-chan struct{} {
    return nil
}
```

## 任务 1.3: core/qrypt/errors.go — 统一错误类型

参考 `docs/core-architecture.md` D6 (L373-397)。

```go
package qrypt

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

func (e *Error) Error() string { ... }
func (e *Error) Unwrap() error { return e.Cause }
```

### vs `internal/drive` 的 sentinel errors

`internal/drive/errors.go` 定义的是 `var ErrNotFound = errors.New(...)` 哨兵错误。
core/ 不做哨兵错误，而是用 `ErrorKind` 枚举 + 结构体，便于调用方（特别是移动端）switch 做本地化处理。

## 任务 1.4: core/qrypt/types.go — 公共类型

### FileEntry

```go
package qrypt

type FileEntry struct {
    ID      string
    Name    string
    DecName string // 解密后的文件名
    IsDir   bool
    Size    int64    // 加密后大小
    PlainSize int64  // 解密后大小
    ModTime time.Time
}

type FileAPIStats struct {
    FileCount int
    DirCount  int
    TotalSize int64
}
```

## 任务 1.5: core/qrypt/upload.go — UploadQueue 接口定义

将 `internal/fs/fs.go:35` 中的 UploadQueue 接口移到 core/（原定义在 `//go:build !nofuse` 保护下，core/ 中去掉 build tag）：

```go
// UploadQueue 异步上传任务队列
type UploadQueue interface {
    Submit(job func(ctx context.Context) error) bool
}
```

## 依赖关系

```
Phase 1 任务无前置依赖，可全部并行。
Phase 2 依赖 Phase 1（接口定义）。
Phase 3 依赖 Phase 1 + Phase 2（接口 + 组件实现）。
```
