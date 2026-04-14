## ADDED Requirements

### Requirement: 路径解析 (Path Resolver)
系统必须支持输入一个人类可读的路径（如 `/Movies/Private`），并逐级将其解析为对应的夸克 `fid`。

#### Scenario: 成功解析子目录
- **WHEN** 用户指定挂载路径为 `/A/B`
- **THEN** 系统从根目录 (0) 开始查找名为 `A` 的目录，再在其下查找 `B` 的 `fid`，并将其作为虚拟根目录

### Requirement: Quark API 身份认证
系统必须支持通过 Cookie 或 Token 进行夸克网盘的身份认证。

#### Scenario: 成功登录
- **WHEN** 用户提供有效的 Cookie 字符串
- **THEN** 系统能够成功获取网盘根目录列表

### Requirement: 获取文件与目录列表
系统必须能够递归或分页获取夸克网盘的文件和文件夹元数据。

#### Scenario: 列出目录内容
- **WHEN** 用户在挂载点执行 `ls` 命令
- **THEN** 系统调用夸克 API 并返回对应的文件列表

### Requirement: 分块下载支持
系统必须支持使用 HTTP Range 头部下载文件的特定字节范围。

#### Scenario: 读取文件片段
- **WHEN** 虚拟文件系统请求读取文件的第 1MB 到 2MB 范围
- **THEN** 驱动层发送带有 `Range: bytes=1048576-2097151` 的请求并返回数据
