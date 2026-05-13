## ADDED Requirements

### Requirement: File modification MUST follow write-back state transitions

系统使用 write-back 策略：文件修改先写入本地 staging 区域，由后台 worker 异步上传。

#### Scenario: Write path
- **WHEN** FUSE `Create` 被调用
- **THEN** 系统在 staging 目录创建对应 fid 的 `.staging` 文件（明文）
- **AND** 初始化一个 24 字节随机 File Nonce 用于后续加密
- **AND** 节点标记为 `isDirty=true`，`source="local"`

#### Scenario: Write and flush
- **WHEN** FUSE `Write` 被调用（常规文件写入）
- **THEN** 数据通过 `staging.WriteAt` 写入 pre-allocated staging 文件
- **AND** 节点 `isDirty=true`
- **AND** 清除 `expectedFid` 防止 `MergeRemoteChanges` 误判

##### Scenario: Sequential edits with staging page buffer
- **WHEN** 同一文件在短时间内收到多次 `Write`
- **AND** staging `Page` 缓冲未满（<1MB）且未到 flush 间隔（250ms）
- **THEN** 写入被合并到内存 `Page` 中，不立即刷盘
- **AND** 距上次写入 250ms 后或缓冲区超 1MB 时自动 flush 到磁盘

#### Scenario: Release triggers sync
- **WHEN** FUSE `Release` / `Fsync` 被调用
- **THEN** staging 通过 `Sync` 确保数据落盘
- **AND** 调用 `enqueueSyncDelay(node, 200ms)` 延迟 200ms 后入队上传

### Requirement: Upload throttle with re-enqueue

系统对同一文件的上传间隔施以 10 秒冷却，并在冷却期间的新写入触发重新入队。

#### Scenario: Repeated edits within 10 seconds
- **WHEN** `syncFile` 开始执行
- **AND** 距离该节点 `lastUploadTime` 不足 10 秒
- **THEN** 跳过本次上传，通过 `defer` 在清理阶段检查 `isDirty`
- **AND** 若 `isDirty=true`，直接向 `uploadChan` 重新入队（保持 `syncQueued=true`）

### Requirement: Successful sync MUST clear dirty markers

上传成功后，系统清除脏状态并更新节点元数据。

#### Scenario: Sync success cleanup
- **WHEN** 上传完成（UpdateHash 或 UploadFinish 成功）
- **THEN** 更新节点 `fid`、`baseServerMtime`、`baseServerSize`、`lastUploadTime`
- **AND** 清除 `isDirty=false`、`uploadID=""`、`lastPart=0`
- **AND** 从 staging 目录移除 `.staging` 文件
- **AND** 从 `pendingNodes` 列表中移除该记录

### Requirement: Sync failure with retry

上传失败后保留足够本地状态用于重试。

#### Scenario: Sync failure and retry
- **WHEN** 上传失败（网络错误、API 拒绝等）
- **THEN** worker 递增重试计数，以指数退避（2s → 4s → 8s → 16s → 32s）重新入队
- **AND** 重试用尽后保留 `.staging` 文件和 `isDirty=true` 状态
- **AND** 从 `pendingNodes` 移除记录防止重复恢复

### Requirement: Startup recovery of dirty files

系统启动时必须扫描 `CacheManager` 中的 `pendingNodes`，对仍然有效的 dirty 节点重新入队。

#### Scenario: Recovery on mount
- **WHEN** `NewFS` 调用 `recoverDirtyFiles`
- **THEN** 对每个 pending 节点，检查其 staging 文件是否仍然存在
- **AND** 若存在且有效，重建 Node 并重新入队上传
- **AND** 若 staging 文件已丢失且节点不是 local_ 前缀，移除 pending 记录

### Requirement: Orphan staging cleanup on startup

系统启动时必须扫描 staging 目录，清理无对应活跃节点的 `.staging` 文件。

#### Scenario: Orphan file cleanup
- **WHEN** `NewFS` 完成节点恢复
- **AND** staging 目录中存在 `.staging` 文件，但其 fid 不在任何活跃节点的 `fidNodes` 中
- **THEN** 删除该孤儿文件，并记录清理日志
