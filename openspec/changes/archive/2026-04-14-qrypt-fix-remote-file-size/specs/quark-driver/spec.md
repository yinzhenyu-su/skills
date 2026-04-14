## MODIFIED Requirements

### Requirement: 获取文件与目录列表
系统必须能够递归或分页获取夸克网盘的文件和文件夹元数据。系统 SHALL 正确解析 API 返回的 `file_size` 字段作为文件的原始字节大小，以确保元数据同步的准确性。

#### Scenario: 列出目录内容
- **WHEN** 用户在挂载点执行 `ls` 命令
- **THEN** 系统调用夸克 API 并返回对应的文件列表
- **AND** 文件列表中所有非空文件的 `Size` 属性应当反映真实的字节数而非 0
