## Why

qrypt 当前强绑定夸克网盘 API——`internal/fs/QryptFS` 直接依赖 `*quark.FileService`、`*quark.ManageService`、`*quark.UploadService` 等具体类型。这导致：
- 无法接入其他云盘（百度网盘、阿里云盘、S3 等）
- 单元测试依赖真实 Quark API，无法 mock
- 加密逻辑（crypt）与存储逻辑（quark）耦合在 FUSE 层

借鉴 Alist 经过 60+ storage backend 验证的 composable interface 模式，将存储后端抽象为统一的 `Driver` 接口，qrypt 核心只依赖接口，具体后端作为实现注入。

## What Changes

- **新增 `internal/drive/` 包**：定义 `Driver` 接口（composable: `Meta` + `Reader` + `Writer` + `Uploader`）、通用 `Entry` 类型、错误类型
- **新增 `internal/drive/quark/` 包**：将 `internal/quark/` 的 API 调用迁移为 `Driver` 接口的 Quark 实现，对外只暴露 `Driver`
- **精简 `internal/quark/`**：降级为纯 HTTP client 层，不再包含业务逻辑
- **修改 `internal/fs/`**：`QryptFS` 构造函数接收 `drive.Driver` 接口而非具体服务指针
- **修改 `internal/sync/uploader.go`**：依赖 `drive.Driver` 而非具体 UploadService
- **新增 `internal/drive/localfs/`**（可选）：测试用本地文件系统实现，不依赖真实 API
- **移除或降级 `quark.CacheService`**：目录/URL/负缓存逻辑分离到通用缓存中间件或归入 drive/quark 内部

## Capabilities

### New Capabilities
- `drive-abstraction`: 统一存储后端接口定义（Driver interface + Entry 类型 + 错误体系）。所有文件系统操作（List、Read、Mkdir、Move、Rename、Remove、Put）通过接口表达，具体后端通过依赖注入接入。

### Modified Capabilities
- `quark-driver`: 现有 quark-driver spec 中的 API 调用行为不变，但实现方式从直接暴露 Service 类型改为实现 `drive.Driver` 接口。所有 Quark 特有的认证、签名、分片上传、秒传逻辑封装在 `drive/quark/` 内部，对外透明。

## Impact

- `internal/drive/` — **新增** ~150 行（接口 + 类型 + 错误定义）
- `internal/drive/quark/` — **新增** ~600 行（从 `internal/quark/` 迁移+适配）
- `internal/drive/localfs/` — **新增** ~100 行（测试用）
- `internal/quark/` — **缩减**：API 调用逻辑移到 `drive/quark/`，保留纯 HTTP client
- `internal/fs/` — **修改**：`QryptFS` 依赖 `drive.Driver` 而非具体服务；Node tree / staging / writeback buffer 保持不变
- `internal/sync/uploader.go` — **修改**：上传入口改为调用 `drive.Driver.Put()`
- 依赖无变化
