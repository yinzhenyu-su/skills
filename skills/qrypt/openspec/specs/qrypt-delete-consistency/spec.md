## ADDED Requirements

### Requirement: Delete operation MUST converge local and remote state

系统执行删除时，必须通过 `metadataOpChan` 将删除操作异步提交给 `metadataWorker` 处理。实际的远程 API 调用由 `metadataWorker` 批量处理（最多等待 200ms 或累积到 100 个 DELETE 任务）。

#### Scenario: Delete with active deletions tracking
- **WHEN** 用户发起删除请求（Unlink / Rmdir）
- **THEN** 系统将目标 fid 存入 `activeDeletions`，状态为 `apiDone=false`
- **AND** 将删除任务提交到 `metadataOpChan`
- **AND** 同步地从内存 VFS 树中移除该节点（`deleteNodePath`）

#### Scenario: Remote delete succeeds
- **WHEN** `processBatchMetadataTasks` 执行批量删除并成功
- **THEN** 系统从 `activeDeletions` 移除该 fid
- **AND** 清理相关的 pending 状态和 cache 条目
- **AND** 在 `ops_log` 中将该操作标记为 `DONE`

#### Scenario: Remote delete fails
- **WHEN** 远程删除 API 返回错误
- **THEN** 系统保留 `activeDeletions` 中的状态以便重试（最多 3 次）
- **AND** 保留本地已删除的 VFS 树状态（用户已感知删除成功）
- **AND** 重试用尽后从 `activeDeletions` 移除（放弃同步）

### Requirement: Hierarchy tracking for deletions

系统必须通过 `deletionsByParent` 按父目录 fids 索引所有活跃删除，通过 `isUnderDeletingDir` 检测路径是否处于被删除的子树中。

#### Scenario: Orphan prevention
- **WHEN** 父目录被删除后，系统发起对该目录下某 dirty 文件的上传
- **THEN** `syncFile` 通过 `isUnderDeletingDir` 检测到父目录正在被删除
- **AND** 放弃此次上传，防止在云端产生"孤儿 Fid"

### Requirement: Recovery MUST skip orphan pending records

系统在 `recoverDirtyFiles` 启动恢复时，对引用不存在的节点或 staging 文件的记录进行清理。

#### Scenario: Orphan pending record on startup
- **WHEN** 启动恢复时发现一个 pending 记录引用了一个不存在的 `Node`
- **AND** 该记录既不是 local_ 前缀 fid、也没有有效的 staging 文件
- **THEN** 系统从 pending 列表中移除该记录
