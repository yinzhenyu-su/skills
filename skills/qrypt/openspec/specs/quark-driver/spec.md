## ADDED Requirements

### Requirement: Quark API 身份认证
系统必须支持通过 Cookie 进行夸克网盘的身份认证，并支持 API 响应中的 `__puus` 凭证自动续期。

#### Scenario: 成功登录
- **WHEN** 用户提供有效的 Cookie 字符串
- **THEN** 系统能够成功获取网盘根目录列表

#### Scenario: Cookie 自动续期
- **WHEN** API 响应返回新的凭据（如 `puus`）
- **THEN** 系统自动更新内存中的 Cookie 字符串，确保长效会话不中断

### Requirement: 获取文件与目录列表
系统必须能够分页获取夸克网盘的文件和文件夹元数据。系统 SHALL 正确解析 API 返回的 `file_size` 字段作为文件的原始字节大小，以确保元数据同步的准确性。

#### Scenario: 列出目录内容
- **WHEN** 用户执行 `qrypt ls` 命令或 FUSE 触发 Readdir
- **THEN** 系统调用夸克 `/file/sort` API 并返回对应的文件列表
- **AND** 当目录文件数超过 100（单页上限）时，系统并发抓取所有后续页面
- **AND** 文件列表中所有非空文件的 `Size` 属性应当反映真实的字节数而非 0

### Requirement: 路径解析 (Path Resolver)
系统必须支持输入一个人类可读的路径（如 `/Movies/Private`），并逐级将其解析为对应的夸克 `fid`。

#### Scenario: 成功解析子目录
- **WHEN** 用户指定挂载路径为 `/A/B`
- **THEN** 系统从根目录 (0) 开始查找名为 `A` 的目录，再在其下查找 `B` 的 `fid`，并将其作为虚拟根目录

### Requirement: 分块下载支持
系统必须支持使用 HTTP Range 头部下载文件的特定字节范围，并且对下载 URL 进行缓存（10 分钟 TTL）以减少重复 API 调用。

#### Scenario: 读取文件片段
- **WHEN** 虚拟文件系统请求读取文件的第 1MB 到 2MB 范围
- **THEN** 驱动层发送带有 `Range: bytes=1048576-2097151` 的请求并返回数据
- **AND** 获取到的 OSS 下载地址在 10 分钟内被缓存，避免重复调用

#### Scenario: 下载遇到 403
- **WHEN** OSS 下载 URL 返回 403 过期错误
- **THEN** 系统自动刷新 URL 缓存并重新获取下载地址后重试

### Requirement: API 韧性与多基地址回退
驱动必须支持基地址冗余。管理接口（Delete/Rename/Move）在遇到 DNS 错误、404 或 405 时，通过 `shouldTryNextMgmtBase` 判断并自动将基地址从 `drive.quark.cn` 切换至 `drive.quark.cn/api/v2` 重试。

### Requirement: 请求类型路由
驱动按请求路径前缀区分请求类型，分配不同的并发信号量：
- 管理路径（`/file/delete`, `/file/rename`, `/file/move`）→ `mgmtSem`（500 并发）
- 元数据路径（`/file/list`, `/file/sort`, `/file/search`）→ `metaSem`（500 并发）
- 其他路径（上传、下载、认证）→ `sem`（200 并发）

### Requirement: 认证签名与 OSS 上传
系统必须遵循夸克 OSS 授权协议，支持分片上传全流程：

#### Scenario: 分片上传生命周期
- **WHEN** 用户创建或修改一个文件
- **THEN** 系统调用 `/file/upload/pre` 获取上传会话（OSS Bucket, ObjKey, UploadId）
- **AND** 使用 `x-oss-user-agent: aliyun-sdk-js/6.6.1` 头部进行 OSS 鉴权
- **AND** 按顺序上传分片，每次上传前通过 `/file/upload/auth` 获取临时签名
- **AND** 上传完成后调用 `UpdateHash` 上报加密对象哈希
- **AND** 如果 `UpdateHash` 返回 `finish=true` 则上传完成；否则继续 `UploadCommit` + `UploadFinish`

#### Scenario: 秒传 (Dedup)
- **WHEN** `UploadPre` 返回秒传成功
- **THEN** 系统验证该 Fid 对应的明文文件名与当前请求一致
- **AND** 若不一致（Hash 碰撞或陈旧 Fid），删除该 Fid 并强制重新上传
