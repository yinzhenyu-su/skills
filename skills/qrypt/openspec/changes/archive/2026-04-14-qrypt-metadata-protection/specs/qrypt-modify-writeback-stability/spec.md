## MODIFIED Requirements

### Requirement: 同步锁机制
系统 SHALL 为每个正在同步的文件节点分配互斥锁，防止同一文件的两个同步任务重叠，并确保上传元数据的原子性更新。在 `Flush` 或 `Release` 触发同步排队时，系统 SHALL 检查该节点是否已经在同步队列中或正在同步，以避免重复排队。

#### Scenario: 自动重试同步
- **WHEN** 针对节点 A 的同步任务正在执行或已在队列中
- **THEN** 此时触发 `Flush` SHALL 不会产生重复的 `syncTask` 进入 `uploadChan`
- **AND** 系统 SHALL 不会重复记录 "queued for upload" 日志
