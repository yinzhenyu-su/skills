## ADDED Requirements

### Requirement: Staging File Lifecycle Management
系统使用 staging 目录存储正在上传或等待上传的文件明文内容。每个活跃文件对应一个 `.staging` 文件，文件名格式为 `<fid>.staging`。

#### Staging 文件创建
- **WHEN** FUSE `Create` 被调用创建新文件
- **THEN** `staging.Store.Create(fid)` 在 staging 目录下创建 `<fid>.staging` 文件
- **AND** `Node.localPath` 指向该 staging 文件路径

#### Staging 写入：Page Buffer 模式
- **WHEN** FUSE `Write` 写入数据到 staging 文件
- **THEN** 数据首先写入内存中的 `Page` 缓冲（每个 fid 一个 Page）
- **AND** 距上次写入 250ms 后或缓冲区超 1MB 时，Page 自动 flush 到磁盘
- **AND** flush 完成后写入从磁盘 staging 文件读取

#### Staging 写入：Direct Fallback 模式
- **WHEN** 文件不在 Page 缓冲区中（如恢复流程中的读取）
- **THEN** 直接通过 `os.File` 写入磁盘 staging 文件
- **AND** `Sync(path)` 调用 `fsync` 确保数据落盘

#### Staging 文件删除（上传成功）
- **WHEN** `syncFile` 上传完成且 `isDirty` 已清除
- **THEN** `staging.Remove(localPath)` 删除 staging 文件
- **AND** 同时从 `pendingNodes` 中移除该记录

#### Staging 文件保留（上传失败）
- **WHEN** 上传重试用尽后仍然失败
- **THEN** staging 文件和 `isDirty=true` 状态被保留
- **AND** 仅清除 `pendingNodes` 中的记录，防止启动恢复时重复重试

### Requirement: 启动时的孤儿文件清理
系统启动时必须扫描 staging 目录，删除无对应活跃节点的 `.staging` 文件。

#### Scenario: Startup orphan cleanup
- **WHEN** `NewFS` 完成节点恢复后
- **THEN** `staging.CleanupOrphanedStagingFiles(activeFids)` 被调用
- **AND** 对每个 `.staging` 文件，检查其 fid 是否存在于任何活跃节点的 fidNodes 映射中
- **AND** 若不存在，删除该孤儿文件并记录清理日志
