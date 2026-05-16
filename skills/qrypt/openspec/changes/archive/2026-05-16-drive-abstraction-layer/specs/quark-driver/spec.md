## MODIFIED Requirements

### Requirement: Quark API 身份认证
系统 SHALL 继续支持通过 Cookie 进行夸克网盘的身份认证，并支持 API 响应中的 `__puus` 凭证自动续期。该逻辑封装在 `drive/quark/` 内部，对外只通过 `Meta.Init()` 暴露初始化接口。

#### Scenario: 成功初始化
- **WHEN** Quark driver 的 `Init(ctx)` 被调用
- **THEN** 系统使用配置中的 Cookie 进行身份验证
- **AND** 如果验证失败，返回错误

#### Scenario: Cookie 自动续期
- **WHEN** API 响应返回新的凭据（如 `__puus`）
- **THEN** 系统自动更新内存中的 Cookie 字符串，确保长效会话不中断
- **AND** 该逻辑对 Driver 接口使用者透明

### Requirement: 获取文件与目录列表
系统 SHALL 继续支持分页获取夸克网盘的文件和文件夹元数据。实现从 `FileService.ListFiles` 迁移到 `drive.Driver.List()`，返回 `[]drive.Entry` 而非 `[]quark.File`。

#### Scenario: 列出目录内容
- **WHEN** 调用 `List(ctx, parentFid)`
- **THEN** 系统调用夸克 `/file/sort` API 并返回 `[]drive.Entry`
- **AND** 当目录文件数超过 100 时，系统并发抓取所有后续页面
- **AND** 所有非空文件的 `Size` 属性应当反映真实的字节数

### Requirement: 分块下载支持
系统 SHALL 继续支持使用 OSS 下载 URL 读取文件特定字节范围，并对下载 URL 进行缓存。实现从 `quark.CacheService` 迁移到 `drive/quark/` 内部缓存。

#### Scenario: 读取文件片段
- **WHEN** `Read(ctx, entry, offset, size)` 被调用
- **THEN** 驱动层获取 OSS 下载 URL，发出带 `Range` 头的 HTTP 请求并返回数据
- **AND** OSS 下载地址在 10 分钟内被缓存，避免重复调用

#### Scenario: 下载遇到 403
- **WHEN** OSS 下载 URL 返回 403 过期错误
- **THEN** 系统自动刷新 URL 缓存并重新获取下载地址后重试

### Requirement: API 韧性与多基地址回退
驱动 SHALL 继续支持基地址冗余。管理接口在遇到 DNS 错误、404 或 405 时自动切换基地址重试。该逻辑封装在 `drive/quark/` 内部。

### Requirement: 认证签名与 OSS 上传
系统 SHALL 继续支持夸克分片上传全流程。该流程封装在 `drive/quark/` 的 `Put()` 方法内部，对外只暴露 `Uploader.Put(ctx, parentID, name, size, body)`。

#### Scenario: 分片上传生命周期
- **WHEN** `Put(ctx, parentID, name, size, body)` 被调用
- **THEN** 驱动内部依次调用 PreUpload → UploadAuth → UploadPart（×N）→ UpdateHash → UploadCommit → UploadFinish
- **AND** 使用 `x-oss-user-agent: aliyun-sdk-js/6.6.1` 头部进行 OSS 鉴权

#### Scenario: 秒传 (Dedup)
- **WHEN** 上传预处理 (`/file/upload/pre`) 返回秒传成功
- **THEN** 系统验证该 Fid 对应的明文文件名与当前请求一致
- **AND** 若不一致，删除该 Fid 并强制重新上传

## ADDED Requirements

### Requirement: 路径解析 (Path Resolver)
系统 SHALL 通过 `List` 逐级查找实现路径到 ID 的解析，供 `QryptFS` 的 `lookup` 操作使用。

#### Scenario: 解析子目录
- **WHEN** 需要将 `/A/B` 解析为对应的 fid
- **THEN** 从根 ID 开始调用 `List` 查找名为 `A` 的目录，再在其下查找 `B` 的 ID
