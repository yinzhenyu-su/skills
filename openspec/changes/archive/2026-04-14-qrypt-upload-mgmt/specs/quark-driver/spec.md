## ADDED Requirements

### Requirement: 文件夹创建 (CreateDir)
驱动层必须支持向夸克 API 发送创建文件夹请求。

#### Scenario: 驱动调用 CreateDir
- **WHEN** VFS 请求在路径 `/A` 下创建子目录 `B`
- **THEN** 驱动向 `/api/v2/file/create_dir` 发送包含 `pdir_fid` 和 `dir_name` 的请求

### Requirement: 删除操作 (Delete)
驱动层必须支持批量或单体文件/文件夹删除。

#### Scenario: 驱动调用 Delete
- **WHEN** VFS 请求删除 ID 为 `fid_123` 的对象
- **THEN** 驱动向 `/api/v2/file/delete` 发送包含 `fids` 数组的请求

### Requirement: 分块上传 (Multipart Upload)
驱动层必须完整实现分块上传流程，包括初始化、获取 AuthKey、上传分块和提交。

#### Scenario: 成功提交分块上传
- **WHEN** 驱动调用 `UploadFinish` 并带有正确的 `task_id` 和 `obj_key`
- **THEN** 夸克 API 返回成功，并将文件挂载到指定目录
