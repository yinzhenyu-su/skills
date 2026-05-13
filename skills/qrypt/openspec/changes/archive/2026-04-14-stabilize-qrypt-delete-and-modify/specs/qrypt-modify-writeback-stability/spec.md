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
