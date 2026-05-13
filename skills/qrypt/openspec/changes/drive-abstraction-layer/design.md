## Context

qrypt 当前架构中，`internal/fs/QryptFS` 直接持有 `*quark.FileService`、`*quark.ManageService`、`*quark.UploadService`、`*quark.CacheService` 四个具体类型的指针。FUSE 操作（readdir、read、write、rename、delete）直接调用这些 service 的方法，返回 Quark 特有的 `quark.File` 类型。

`internal/sync/uploader.go` 同样直接依赖 `*quark.FileService`、`*quark.UploadService` 等。

这种强耦合导致：
- 无法在不修改 `fs/` 代码的前提下接入第二个云盘
- 单元测试必须 mock 四个 service 接口
- Quark API 响应结构（`SortResp`、`UpPreResp` 等）渗透到整个调用栈

Alist 经过 60+ storage backend 验证的 composable interface 模式是成熟的参考范本。

## Goals / Non-Goals

**Goals:**
- 定义统一的 `drive.Driver` 接口，覆盖 qrypt 需要的所有存储操作
- 将现有 Quark 实现迁移为 `drive/quark.Driver`，对外只暴露 `drive.Driver`
- `internal/fs/QryptFS` 只依赖 `drive.Driver` 接口，不依赖具体实现
- `internal/sync/uploader.go` 使用 `drive.Driver.Put()` 上传
- 保留所有现有功能不变，不改写 FUSE 节点树、staging、writeback buffer

**Non-Goals:**
- 不改加密层（`internal/crypt/`）——加密仍在 FUSE 层调用
- 不做 WebDAV 或 FUSE 以外的访问协议
- 不添加新云盘支持（本 change 只做抽象，第二个云盘是后续 change）
- 不断点续传的 SessionedUploader 接口
- 不改变 CLI 命令或用户配置格式

## Decisions

### D1: Composable interface（组合接口，非大接口）

借鉴 Alist 的 `Driver` + `Reader` + `Writer` + `Uploader` 组合模式：

```go
// internal/drive/driver.go

package drive

import (
    "context"
    "io"
    "time"
)

// Entry 是统一的文件/目录条目，所有 driver 返回同一类型
type Entry struct {
    ID       string
    Name     string
    IsDir    bool
    Size     int64
    ModTime  time.Time
    Extra    any  // driver 特定元数据透传（如 Quark 的 category）
}

// Meta 是所有 driver 必须实现的生命周期接口
type Meta interface {
    Init(ctx context.Context) error
    Drop(ctx context.Context) error
}

// Reader 是只读操作接口（最小必需能力）
type Reader interface {
    List(ctx context.Context, parentID string) ([]Entry, error)
    Read(ctx context.Context, entry Entry, offset, size int64) (io.ReadCloser, error)
}

// Writer 是写操作接口（可选能力）
type Writer interface {
    Mkdir(ctx context.Context, parentID, name string) (Entry, error)
    Move(ctx context.Context, entry Entry, dstParentID string) error
    Rename(ctx context.Context, entry Entry, newName string) error
    Remove(ctx context.Context, entry Entry) error
}

// Uploader 是上传接口（可选能力）
type Uploader interface {
    Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (Entry, error)
}

// Driver 组合最小能力，具体实现按需实现更多接口
type Driver interface {
    Meta
    Reader
}
```

选择依据：
- 这样 `QryptFS` 可以按需做类型断言：如果 driver 也实现了 `Writer`，才启用写操作
- 测试时可以用只实现了 `Meta` + `Reader` 的 mock（无需 mock 上传）
- 加新 driver 时不需要实现用不到的方法

### D2: Entry 替代 quark.File

所有 driver 的 `List()` 返回 `[]Entry`，取代当前的 `[]quark.File`。

```go
type Entry struct {
    ID       string
    Name     string
    IsDir    bool
    Size     int64
    ModTime  time.Time
    Extra    any  // driver 特定数据
}
```

`quark/driver.go` 内部做 `quark.File → Entry` 转换。FUSE 层的 `fs.Node` 仍然存在（持有 dirty 标记、nonce、sync 状态等），Node 中的 `fid` / `size` / `mtime` 改为根据 `Entry` 更新。

### D3: Put-only 上传接口

```go
type Uploader interface {
    Put(ctx context.Context, parentID, name string, size int64, body io.Reader) (Entry, error)
}
```

不设计 `SessionedUploader`。理由：
- qrypt 的 staging 机制意味着上传中断时源文件仍在本地磁盘，crash 恢复后重传整个文件
- 不需要跨进程断点续传（`recoverDirtyFiles` 从 staging 重新读取上传）
- 不需要上传进度 UI（qrypt 是 CLI/FUSE，非 Web 应用）
- 统一接口降低所有 driver 的实现负担

Quark driver 的 `Put()` 内部走现有的六步上传流程（PreUpload → Auth → UploadPart → Hash → Commit → Finish），对外透明。

### D4: CacheService 归入 driver 内部

当前的 `quark.CacheService`（dirCache、urlCache、negCache）与 Quark API 响应类型（`[]File`）强绑定。抽象后：
- 这些缓存逻辑归入 `drive/quark/internal`，作为 Quark driver 的内部实现细节
- 不设计公共缓存接口
- 不迁移到通用缓存层（避免不必要的抽象）

如果后续多个 driver 都需要目录缓存，再提取公共缓存中间件。

### D5: 注册机制

不使用 Alist 的 `init()` + side-effect import 模式。qrypt 是单二进制，编译时确定 driver。采用手动实例化 + 接口注入：

```go
// internal/fs/fs.go  —— 构造方式变化
func NewFS(
    ctx context.Context,
    drv drive.Driver,        // ← 接口，不再需要四个具体 service
    cipher *crypt.RcloneCipher,
    cacheMgr *cache.CacheManager,
    rootID string,
    opts FSOptions,
) *QryptFS { ... }
```

保留扩展性：如果要动态选择 driver，可以在配置层加工厂函数：

```go
// drive/quark/driver.go
func NewQuarkDriver(cookie, rootPath string) drive.Driver { ... }
```

### D6: 加密层保持现状

`internal/crypt/` 不做改动。`QryptFS` 继续在 FUSE 操作层调用 cipher（`EncryptSegment` 加密文件名、`EncryptBlock` 加密内容块）。Drive 接口不感知加密，它只读写已经加密后的数据。

这一决定跟 Alist 不同（Alist 的 crypt 是包裹另一个 Driver 的装饰器），原因是 qrypt 的加密跟 FUSE 块读写深度耦合（64KB block、随机访问解密），不适合在 Drive 层做透明加解密。

## Risks / Trade-offs

| 风险 | 缓解措施 |
|------|---------|
| **Quark upload 六步流程封装进 Put() 后，错误信息不够细化** | `Put()` 返回 `(Entry, error)`，error 包裹底层错误（使用 `fmt.Errorf("upload pre: %w", err)` 链式包装），调用方可判断错误类型 |
| **List 从 `[]quark.File` 改为 `[]Entry`，丢失 File 特有字段（如 Category）** | Entry.Extra 字段透传。FUSE 层如有需求可断言 `entry.Extra.(quarkFileMeta)` 获取 |
| **QryptFS 修改范围预估不准** | 先做 `drive/driver.go` + `drive/quark/driver.go`，再逐个修改 `fs/` 中的引用点，每改一个编译一次验证 |
| **Put(body io.Reader) 不支持进度反馈** | 当前不需要。如需进度，后续添加可选的 `ProgressReader` 包装或 callback 参数（不破坏兼容性） |

## Migration Plan

1. **Phase 1 - 定义接口**：创建 `internal/drive/driver.go`（`Entry` + `Meta` + `Reader` + `Writer` + `Uploader` + `Driver`）
2. **Phase 2 - Quark 实现**：创建 `internal/drive/quark/driver.go`，将现有的 `FileService.ListFiles` / `ManageService.CreateDir/Delete/Rename/Move` / `UploadService.*` + `client.go` 迁移为 `drive.Driver` 实现
3. **Phase 3 - FUSE 接入**：修改 `internal/fs/fs.go` 构造函数，将 `*quark.XXXService` 参数替换为 `drive.Driver`；逐步替换 `fs/` 中的调用点
4. **Phase 4 - Sync 接入**：修改 `internal/sync/uploader.go` 使用 `drive.Driver.Put()`
5. **Phase 5 - 清理**：确认所有功能正常后，标记 `internal/quark/` 中不再使用的代码为 deprecated，但不急于删除（避免破坏性变更）
