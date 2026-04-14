## ADDED Requirements

### Requirement: Node-based synchronization following renames
The system SHALL identify and lock files for synchronization based on their unique identity (e.g., FID or an internal unique ID) rather than their filesystem path.

#### Scenario: Sync continues after rename
- **WHEN** a file `/data.bin` is queued for upload and then renamed to `/archive.bin` before or during sync
- **THEN** the system MUST ensure that only one sync task proceeds for the node and that the sync completes using the latest known path/name

### Requirement: Sync task resolution by node
The background upload worker SHALL resolve sync tasks by looking up the current node state at the time of execution, ensuring that path changes do not break the sync pipeline.

#### Scenario: Retry follows rename
- **WHEN** a sync for `/old.txt` fails and is scheduled for retry, but the file is renamed to `/new.txt` in the meantime
- **THEN** the retry task MUST correctly identify the node at `/new.txt` and resume synchronization

## MODIFIED Requirements

### Requirement: Sync failure MUST remain retryable
On synchronization failure, the system MUST preserve the `isDirty` state of the node to ensure it remains eligible for subsequent sync attempts. The `isDirty` flag SHALL only be cleared upon confirmed successful upload finalization.

#### Scenario: Sync failure and retry
- **WHEN** upload sync fails at any stage (pre-upload, part upload, hash update, or commit)
- **THEN** the system MUST ensure the `isDirty` flag remains `true` and the file is kept in the pending state for retry

### Requirement: 同步锁机制
系统 SHALL 为每个正在同步的文件**节点**（以 FID 或唯一 ID 标识）分配互斥锁，防止同一文件的两个同步任务重叠，无论其路径是否发生变化。

#### Scenario: 自动重试同步
- **WHEN** 上一次针对节点 A 的同步因为网络失败
- **THEN** 系统重试时应当首先检查该**节点**是否已经在同步中，避免同一节点产生竞争任务
