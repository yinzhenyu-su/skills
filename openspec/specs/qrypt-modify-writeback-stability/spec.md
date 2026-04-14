## ADDED Requirements

### Requirement: File modification MUST follow write-back state transitions

The system MUST track modified files as dirty after write and MUST enqueue synchronization on flush/release-equivalent completion points.

#### Scenario: File modified and flushed

- **WHEN** a file receives write operations and then flush is triggered
- **THEN** the system MUST mark the file dirty and enqueue a sync task

### Requirement: Upload sync MUST prefer hash completion before commit fallback

After multipart upload, the system MUST report encrypted object hashes and MUST treat finish=true as a successful terminal state; only when finish is false MAY it fallback to commit and upload finish APIs.

#### Scenario: Hash path completes upload

- **WHEN** multipart parts are uploaded and hash update returns finish=true
- **THEN** the system MUST finalize file state without requiring commit

#### Scenario: Hash path requires fallback

- **WHEN** multipart parts are uploaded and hash update returns finish=false
- **THEN** the system MUST execute commit and upload finish flow before finalizing file state

### Requirement: Successful sync MUST clear dirty and pending markers

On successful upload finalization, the system MUST clear dirty markers and pending records, and MUST update node metadata to the latest fid/size/encryption nonce state.

#### Scenario: Sync success cleanup

- **WHEN** a file sync completes successfully
- **THEN** the system MUST clear pending/dirty state and persist the updated node metadata

### Requirement: Sync failure MUST remain retryable

On synchronization failure, the system MUST preserve enough local state for retry and MUST NOT falsely report completion.

#### Scenario: Sync failure and retry

- **WHEN** upload sync fails at hash, commit, or finish stage
- **THEN** the system MUST keep pending state and schedule or allow retry without data loss

### Requirement: 并发写入一致性
当文件处于同步上传状态（`syncing`）时，系统必须协调新的 `Write` 请求。在同步开始前，系统应当对文件的脏分块索引进行快照，或者在同步过程中阻塞冲突块的修改。

#### Scenario: 同步中的并发写入
- **WHEN** 正在后台上传 `file.dat` 的分块 5, 6, 7
- **THEN** 此时对分块 8 的写入应当排队或成功记录，且不会破坏正在进行的上传

### Requirement: 同步锁机制
系统 SHALL 为每个正在同步的文件节点分配互斥锁，防止同一文件的两个同步任务重叠，并确保上传元数据的原子性更新。

#### Scenario: 自动重试同步
- **WHEN** 上一次同步因为网络失败
- **THEN** 系统重试时应当首先检查该文件是否已经在队列中，避免重复创建上传任务
