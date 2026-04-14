## MODIFIED Requirements

### Requirement: File modification MUST follow write-back state transitions

The system MUST track modified files as dirty after write, MUST persist the modified plaintext into a dedicated file-level write-back staging source, and MUST enqueue synchronization on flush/release-equivalent completion points without depending on decrypted read-cache chunk state.

#### Scenario: File modified and flushed

- **WHEN** a file receives write operations and then flush is triggered
- **THEN** the system marks the file dirty, updates its file-level staging source, and enqueues a sync task

### Requirement: Successful sync MUST clear dirty and pending markers

On successful upload finalization, the system MUST clear dirty markers and pending records for the synchronized snapshot, MUST update node metadata to the latest fid/size/encryption nonce state, and MUST release file-level staging state that is no longer needed for retry while preserving any newer local modifications.

#### Scenario: Sync success cleanup

- **WHEN** a file sync completes successfully
- **THEN** the system clears pending and dirty state for the uploaded snapshot, persists updated node metadata, and removes completed staging state for that file only when no newer write has made the file dirty again

### Requirement: Sync failure MUST remain retryable

On synchronization failure, the system MUST preserve enough file-level local state for retry, including staged plaintext and pending metadata needed to locate the latest file identity and visible path, and MUST NOT falsely report completion.

#### Scenario: Sync failure and retry

- **WHEN** upload sync fails at hash, multipart upload, commit, or finish stage
- **THEN** the system keeps file-level pending state and local staged plaintext required to schedule or allow retry without data loss

### Requirement: 同步锁机制

系统 SHALL 为每个正在同步的文件节点分配基于稳定文件身份的互斥锁，而不是基于瞬时路径字符串去重，防止同一文件在重命名、重试或多路径引用期间出现重叠同步任务，并确保上传元数据更新的原子性。

#### Scenario: 自动重试同步

- **WHEN** 上一次同步因为网络失败后文件在重试前发生重命名
- **THEN** 系统仍然只为该文件保留一个有效同步任务，并使用最新路径元数据继续后续重试
