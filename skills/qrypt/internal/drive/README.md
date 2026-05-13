# Qrypt Driver 开发指南

## 概述

qrypt 的存储后端采用 **可组合接口**（Composable Interface）模式，灵感来自 Alist。
一个驱动只需要实现必要的接口组合即可工作：

- **只读模式**：`Meta` + `Reader` → `Driver`
- **可写模式**：`Meta` + `Reader` + `Writer`
- **上传模式**：`Meta` + `Reader` + `Writer` + `Uploader`

消费者通过类型断言按需发现能力：

```go
if w, ok := drv.(drive.Writer); ok {
    w.Mkdir(ctx, parentID, "newDir")
}
```

## 接口体系

```
Meta              Reader              Writer               Uploader
┌─────────┐      ┌─────────┐        ┌──────────┐         ┌──────────┐
│ Init    │      │ List    │        │ Mkdir    │         │ Put      │
│ Drop    │      │ Read    │        │ Move     │         └──────────┘
└─────────┘      └─────────┘        │ Rename   │
                                    │ Remove   │
                                    └──────────┘
```

### `Meta`（必需）

| 方法 | 说明 | 典型实现 |
|------|------|---------|
| `Init(ctx)` | 一次性的认证/鉴权/初始化 | 调用 API 验证凭据、初始化缓存 |
| `Drop(ctx)` | 清理资源 | 关闭连接池、刷新缓存 |

### `Reader`（必需）

| 方法 | 说明 |
|------|------|
| `List(ctx, parentID)` | 返回目录下所有子条目 |
| `Read(ctx, entry, offset, size)` | 返回文件字节范围 `[offset, offset+size)` 的 Reader |

### `Writer`（可选）

| 方法 | 说明 |
|------|------|
| `Mkdir(ctx, parentID, name)` | 创建目录，返回新条目 |
| `Move(ctx, entry, dstParentID)` | 移动到另一个目录 |
| `Rename(ctx, entry, newName)` | 同目录内重命名 |
| `Remove(ctx, entry)` | 删除文件或目录 |

### `Uploader`（可选）

| 方法 | 说明 |
|------|------|
| `Put(ctx, parentID, name, size, body)` | 上传文件内容到父目录 |

## Entry 约定

`drive.Entry` 是跨层描述文件/目录的统一结构：

```go
type Entry struct {
    ID      string      // 跨层唯一标识（见下方约定）
    Name    string      // 文件名
    IsDir   bool        // 是否目录
    Size    int64       // 文件大小（字节）
    ModTime time.Time   // 修改时间
    Extra   any         // 驱动特有元数据（类型断言获取）
}
```

### ID 约定

ID 是 FUSE 层定位条目的唯一键。不存在跨层"路径"概念——一切通过 ID 寻址。

| 驱动 | ID 格式 | 示例 |
|------|---------|------|
| quark | 服务端 FID 字符串 | `"0"`（根）, `"a1b2c3"` |
| yun139 | 服务端 FileID | `"/"`（根）, `"file:12345"` |
| localfs | 本地文件路径 | `"/tmp/qrypt/a.txt"` |
| quarkmock | 自增 mock ID | `"mock_fid_1"`, `"mock_fid_2"` |

**根目录**：所有驱动应当支持 ID `"0"` 表示根，但若驱动有更自然的根表达方式（如 yun139 的 `rootID`），应在 `Init` 或 `List` 中做回退。

### Extra 字段

`Extra` 用于携带驱动特定的元数据。消费者通过类型断言获取：

```go
type QuarkExtra struct {
    FIDType string
    PID     string
}
if qe, ok := entry.Extra.(QuarkExtra); ok { ... }
```

仅当 FUSE 层或其他消费者需要访问驱动特有信息时使用。大多数情况下不需要。

## 哨兵错误

定义在 `errors.go` 中。驱动应按语义返回对应的哨兵错误，FUSE 层据此映射到系统错误码。

| 错误 | 含义 | 使用场景 |
|------|------|---------|
| `ErrNotFound` | 条目不存在 | `List` `Read` `Remove` 找不到目标 |
| `ErrDirAlreadyExists` | 目录已存在 | `Mkdir` 创建重名目录 |
| `ErrAlreadyExists` | 条目已存在 | `Put` `Rename` 目标已存在 |
| `ErrNotDir` | 路径不是目录 | `List` 目标不是目录 |
| `ErrNotImplemented` | 不支持的操作 | 显式告知调用方 |

**注意**：驱动不应返回通用 `error` 字符串——FUSE 层只能将特定哨兵错误映射到 `-ENOENT`、`-EEXIST` 等系统错误。请根据 API 的返回码映射到对应哨兵。

## 实现步骤

### 第一步：新建包

```bash
mkdir internal/drive/<name>/
```

创建 `driver.go` 作为主入口，按需创建 `client.go`（HTTP 客户端）、`types.go`（API 类型）。

### 第二步：定义结构体

```go
package mydriver

import (
    "context"
    "fmt"
    "io"

    "github.com/yinzhenyu/skills/qrypt/internal/drive"
)

type MyDriver struct {
    cl     *client   // API 客户端
    conf   string    // 驱动参数
}

// 编译期检查——确保实现了所有接口
var (
    _ drive.Driver   = (*MyDriver)(nil)
    _ drive.Writer   = (*MyDriver)(nil)  // 如果实现了 Writer
    _ drive.Uploader = (*MyDriver)(nil)  // 如果实现了 Uploader
)

func NewDriver(param1, param2 string) *MyDriver {
    return &MyDriver{
        cl:   newClient(param1),
        conf: param2,
    }
}
```

### 第三步：实现 Init / Drop

```go
func (d *MyDriver) Init(ctx context.Context) error {
    // 验证凭据有效性
    return d.cl.authenticate()
}

func (d *MyDriver) Drop(ctx context.Context) error {
    // 清理：关闭 idle 连接等
    return nil
}
```

### 第四步：实现 List / Read（最低要求）

```go
func (d *MyDriver) List(ctx context.Context, parentID string) ([]drive.Entry, error) {
    // 调用 API 获取目录列表
    // 将服务端类型转换为 drive.Entry
    // 处理好分页
}

func (d *MyDriver) Read(ctx context.Context, entry drive.Entry, offset, size int64) (io.ReadCloser, error) {
    // 获取下载 URL
    // 发起 HTTP Range 请求
    // 返回响应 Body
}
```

### 第五步：按需实现 Writer / Uploader

参考 `quark/driver.go` 或 `yun139/driver.go` 中的模式。

### 第六步：实现 `toEntry()` 转换函数

每个驱动应当有一个内部辅助函数，将 API 返回的类型映射为 `drive.Entry`：

```go
func toEntry(apiItem item) drive.Entry {
    return drive.Entry{
        ID:      apiItem.FileID,
        Name:    apiItem.FileName,
        IsDir:   apiItem.Dir,
        Size:    apiItem.Size,
        ModTime: time.UnixMilli(apiItem.UpdateAt),
    }
}
```

处理批量返回时传入 `[]item`，返回 `[]drive.Entry`。

## 配置注册

新增驱动需要修改 **3 个文件**：

### 1. `internal/config/config.go`——添加 Options 结构体 + DriveConfig 字段

```go
// 在 DriveConfig 中加指针字段
type DriveConfig struct {
    Type   string           `toml:"type"`
    Quark  *QuarkOptions    `toml:"quark"`
    Yun139 *Yun139Options   `toml:"yun139"`
    MyDrv  *MyOptions      `toml:"my"`    // ← 新增
}

// 定义新驱动的配置
type MyOptions struct {
    Token string `toml:"token"`
    Root  string `toml:"root"`
}
```

对应的 TOML 配置：

```toml
[drive]
type = "my"

[drive.my]
token = "..."
root = "/"
```

### 2. `internal/drive/factory/factory.go`——添加 case 分支

```go
func NewDriverFromConfig(cfg config.DriveConfig) (drive.Driver, error) {
    switch cfg.Type {
    case "quark":
        // ...
    case "yun139":
        // ...
    case "my":                            // ← 新增
        if cfg.MyDrv == nil {
            return nil, fmt.Errorf("missing my config")
        }
        return mydriver.NewDriver(cfg.MyDrv.Token, cfg.MyDrv.Root), nil
    default:
        return nil, fmt.Errorf("unknown driver type: %q", cfg.Type)
    }
}
```

### 3. 驱动包本身

```bash
internal/drive/<name>/driver.go     # 主实现
internal/drive/<name>/client.go     # HTTP 客户端（可选）
internal/drive/<name>/types.go      # API 类型（可选）
```

## 测试

### 快速验证：用 localfs 模式

对于不需要服务端的接口验证，可以用 `internal/drive/localfs` 模式直接挂载本地目录运行测试：

```go
drv := localfs.NewDriver("/tmp/testroot")
vfs := fs.NewFS(drv, cipher, cacheMgr, "0", ...)
```

参考 `internal/fs/fs_test.go`。

### 驱动测试：用 quarkmock 模式

对于需要模拟服务端语义的 E2E 测试，用 `quarkmock.NewDriver()` 得到一个全功能的内存 Mock：

```go
drv := quarkmock.NewDriver()
// 支持 List/Read/Mkdir/Move/Rename/Remove/Put
```

参考 `internal/fs/e2e_test.go`。

### 驱动专属测试

在 `internal/drive/<name>/` 目录下创建 `driver_test.go`，推荐：

1. 创建一个 mock HTTP server（`httptest.NewServer`）
2. 用 mock server 的 URL 初始化驱动（构造注入 client）
3. 测试每个接口方法

参考 `internal/drive/quark/` 和 `internal/drive/yun139/` 的测试模式（当前无测试文件——TODO）。

## 参考实现速查

| 驱动 | 文件 | 关注点 |
|------|------|--------|
| **quark** | `driver.go` | 完整参考：认证、缓存、分页、API 错误映射 |
| | `client.go` | Cookie 鉴权、多 BaseURL 故障转移 |
| | `types.go` | API 类型 ↔ Entry 转换、apiError 错误映射 |
| **yun139** | `driver.go` | 多段上传（Init→Upload→Commit）、游标分页、ID 回退 |
| | `client.go` | Authorization Token 鉴权、自动刷新 |
| | `types.go` | 复杂 API 响应结构 |
| **localfs** | `driver.go` | 最简实现：IO 操作包装为 Driver |
| **quarkmock** | `driver.go` | 纯内存 Mock：并发安全、FID 生成、哨兵错误 |

## 设计原则

1. **加密在 FUSE 层**：Driver 接口传输已加密数据。读写的是 ciphertext。
2. **CacheService 在内部**：每个驱动自行管理缓存，不在 Driver 接口暴露。
3. **Put-only 上传**：没有 SessionedUploader。staging 提供崩溃恢复。
4. **ID 驱动**：一切操作通过 ID 寻址，不暴露路径逻辑到接口层。

## ⚠️ 注意事项

### Quark 上传流程：不要把 OSS 操作当成 Quark API

`Put()` 的完整上传流程是：

```
/file/upload/pre  →  OSS PUT parts  →  /file/update/hash  →  OSS CompleteMultipartUpload  →  /file/upload/finish
```

最后两步容易踩坑：
- `/file/upload/commit` **不存在**。如果需要提交分片上传，必须直接 POST 到阿里云 OSS 的 CompleteMultipartUpload（XML body + ETag），而不是调 Quark API。
- `/file/upload/finish` 的请求体只需 `{obj_key, task_id}`，不要传多余字段。
- `/file/update/hash` 的请求体只需 `{md5, sha1, task_id}`，不要传 fid/bucket/upload_id。

参考实现：`quark/driver.go` 中的 `ossComplete()` 方法。
