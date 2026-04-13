## ADDED Requirements

### Requirement: 路径与 FID 持久化
系统必须将已解析的 `Path -> FID` 映射存储在本地 SQLite 数据库中。

#### Scenario: 重启后快速定位
- **WHEN** 用户重启 qrypt 挂载点并进入之前访问过的目录
- **THEN** 系统直接从 SQLite 加载 `fid` 而不发起网盘 API 列表请求

### Requirement: 节点属性持久化
系统必须持久化存储文件的 `size`, `mtime` 和 `rclone nonce` 属性。

#### Scenario: 获取已访问过的文件属性
- **WHEN** 用户执行 `ls -l` 查看目录
- **THEN** 系统从数据库返回精确的 `mtime` 和 `size`
