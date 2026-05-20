## ADDED Requirements

### Requirement: 跨 mount 文件复制

系统 SHALL 支持在不同 mount instance 之间复制文件，使用 `qrypt cp <src_mount>:<src_path> <dst_mount>:<dst_path>` 语法。

#### Scenario: 夸克到夸克跨账号复制
- **WHEN** 执行 `qrypt cp personal:/docs/file.txt work:/backup/`
- **THEN** 系统从 src mount 读取文件（经其 cipher 解密）
- **AND** 经 dst mount 的 cipher 重新加密
- **AND** 写入 dst mount 的指定路径
- **AND** 不写本地磁盘中转

#### Scenario: 网盘到本地文件系统
- **WHEN** 执行 `qrypt cp personal:/docs/file.txt localfs:/tmp/`
- **THEN** 读取、解密、写入本地 localfs driver

#### Scenario: 文件不存在报错
- **WHEN** src path 在 src mount 中不存在
- **THEN** 命令报错结束，不创建 dst 文件

### Requirement: 流式传输管道

系统 SHALL 使用流式管道实现跨盘传输，无需在本地存储完整文件。

#### Scenario: 大文件流式传输
- **WHEN** 复制 2GB 文件
- **THEN** 文件以分块形式从 src reader 流经 decrypt/encrypt 到 dst writer
- **AND** 峰值内存不超过分块大小 + buffer（约 4MB + 加密开销）
