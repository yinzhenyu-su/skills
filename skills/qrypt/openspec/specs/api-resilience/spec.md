## ADDED Requirements

### Requirement: 请求并发量控制
系统必须使用信号量（channel）限制夸克 API 的并发请求数，按请求类型区分：
- 文件/上传请求：最多 200 个并发（`sem`）
- 管理请求（Delete/Rename/Move）：最多 500 个并发（`mgmtSem`），按路径前缀 `/file/delete`、`/file/rename`、`/file/move` 识别
- 元数据请求（List/Sort/Search）：最多 500 个并发（`metaSem`），按路径前缀 `/file/list`、`/file/sort`、`/file/search` 识别

### Requirement: HTTP 重试与指数退避
当 API 返回网络错误或可重试的 HTTP 状态码时，系统必须自动重试（最多 3 次），每次重试使用指数退避 + 随机抖动。

#### Scenario: 网络错误重试
- **WHEN** 夸克 API 请求因网络错误（DNS 解析失败、TLS 握手超时、连接被重置、Broken Pipe 等）失败
- **THEN** 系统按指数退避（500ms → 1s → 2s，叠加随机抖动）重试
- **AND** 最多重试 3 次

#### Scenario: HTTP 429 限流重试
- **WHEN** 夸克 API 返回 429 (Too Many Requests) 状态码
- **THEN** 系统视为可重试状态，应用相同的指数退避策略

#### Scenario: 5xx 服务端错误重试
- **WHEN** 夸克 API 返回 500 及以上状态码
- **THEN** 系统视为可重试状态，应用相同的指数退避策略

### Requirement: 基地址自动轮询
管理接口（Delete/Rename/Move）在遇到 DNS 错误、404 或 405 时，必须自动轮询备用基地址。

#### Scenario: 主地址 404 时切换
- **WHEN** 管理请求发送到 `drive.quark.cn` 返回 404 或 DNS 错误
- **THEN** 系统自动切换到备用地址 `drive.quark.cn/api/v2`（通过 `shouldTryNextMgmtBase` 判断）
- **AND** 使用新的基地址重试请求

### Requirement: Cookie 自动续期
系统必须能根据 HTTP 响应头中的 `Set-Cookie` 实时更新内存中的 Cookie 状态，特别是 `__puus` 凭证。

#### Scenario: 保持登录
- **WHEN** 夸克在 API 响应中提供新的 `__puus`（通过 `Set-Cookie` 头部）
- **THEN** 系统解析 `Set-Cookie` 并调用 `updateCookie("__puus", value)` 更新内存中的 Cookie
- **AND** 后续所有请求自动携带续期后的 Cookie

### Requirement: OSS 上传分片重试
上传分片（UploadPart）和提交（Commit）操作使用独立的 OSS 重试策略，最多重试 3 次。

#### Scenario: OSS 分片上传失败
- **WHEN** 上传分片到 OSS 时遇到网络错误或 HTTP 5xx
- **THEN** 系统按指数退避（500ms → 1s → 2s）重试，最多 3 次
- **AND** 重试耗尽后，UploadPart 向上层返回错误，由 `syncFile` 的 woker 重试机制（指数退避 2s → 4s → ... → 32s，最多 5 次）接管

### Requirement: 下载 URL 超时重试
下载请求使用独立的 `http.Client`（无超时限制），当下载返回 403 时触发 URL 刷新后重试。

#### Scenario: 分块下载 403 重试
- **WHEN** 使用缓存的 OSS 下载 URL 请求分块时返回 403
- **THEN** 系统清除该 URL 缓存并重新从夸克 API 获取最新的下载地址
- **AND** 使用新 URL 重试下载请求
