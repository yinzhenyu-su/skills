## ADDED Requirements

### Requirement: 写入请求拦截 (Write Interception)
FUSE 驱动层必须拦截所有的 `Write` 请求，并将数据异步写入本地缓存系统，同时更新文件的 `is_dirty` 状态。

#### Scenario: 成功写入缓存
- **WHEN** 用户向挂载点中的文件写入数据
- **THEN** FUSE 层直接向 `CacheManager` 请求存储该块，并立即向用户程序返回写入成功

### Requirement: 同步机制 (Flush/Release)
FUSE 驱动层必须在收到 `Flush` 或 `Release` 请求时，同步处理文件的脏数据上传。

#### Scenario: 文件关闭触发同步
- **WHEN** 用户关闭一个已修改的文件句柄
- **THEN** FUSE 层启动异步上传任务，并在上传成功后更新 VFS 内部的文件节点信息

### Requirement: 文件管理操作转发 (Management Proxy)
FUSE 驱动层必须将 `Mkdir`, `Rename`, `Unlink` 等元数据修改操作转发给 `QuarkDriver`。

#### Scenario: 删除操作转发
- **WHEN** FUSE 收到 `Unlink` 请求
- **THEN** 系统调用驱动层的 `Delete` 方法，若成功则从本地 VFS 树中移除该节点
