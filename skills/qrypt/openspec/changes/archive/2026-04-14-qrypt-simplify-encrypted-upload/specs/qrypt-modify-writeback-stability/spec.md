## MODIFIED Requirements

### Requirement: File modification MUST follow write-back state transitions

The system MUST track modified files as dirty after write, MUST persist the modified plaintext into a file-level local staging source, and MUST enqueue synchronization on flush/release-equivalent completion points.

#### Scenario: File modified and flushed

- **WHEN** a file receives write operations and then flush is triggered
- **THEN** the system marks the file dirty, updates its file-level staging source, and enqueues a sync task

### Requirement: Upload sync MUST verify encrypted object hash before commit fallback

After multipart upload, the system MUST report encrypted object hashes and MUST treat `finish=true` as a successful terminal state; only when `finish` is false MAY it execute commit and upload finish APIs.

#### Scenario: Hash path completes upload

- **WHEN** multipart upload succeeds and hash update returns `finish=true`
- **THEN** the system finalizes file state without executing commit fallback

#### Scenario: Hash path requires multipart upload

- **WHEN** multipart upload succeeds and hash update returns `finish=false`
- **THEN** the system executes commit and upload finish flow before finalizing file state

### Requirement: Successful sync MUST clear dirty and pending markers

On successful upload finalization, the system MUST clear dirty markers and pending records, MUST update node metadata to the latest fid/size/encryption nonce state, and MUST release file-level staging state that is no longer needed for retry.

#### Scenario: Sync success cleanup

- **WHEN** a file sync completes successfully
- **THEN** the system clears pending and dirty state, persists updated node metadata, and removes completed staging state for that file

### Requirement: Sync failure MUST remain retryable

On synchronization failure, the system MUST preserve enough file-level local state for retry and MUST NOT falsely report completion.

#### Scenario: Sync failure and retry

- **WHEN** upload sync fails at hash, multipart upload, commit, or finish stage
- **THEN** the system keeps file-level pending state and local staged plaintext required to schedule or allow retry without data loss
