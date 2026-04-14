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

## ADDED Requirements

### Requirement: Modified files MUST be staged as file-level local sources
The system MUST persist each modified file into a single local staging file that can be reopened for hashing and upload, rather than requiring chunk metadata reconstruction to build upload input.

#### Scenario: Create and modify a file
- **WHEN** a user creates or modifies a file in the mounted filesystem
- **THEN** the system stores the modified plaintext in a file-level local staging source associated with that path

#### Scenario: Flush schedules upload from staged file
- **WHEN** flush or release queues synchronization for a modified file
- **THEN** the upload pipeline uses the staged file as the canonical plaintext source for hash computation and upload

### Requirement: Encryption MUST be exposed as a sequential byte stream
The system MUST transform a plaintext staging file into an rclone-compatible encrypted byte stream during upload, including the file header and sequential encrypted content blocks.

#### Scenario: Upload starts from plaintext source
- **WHEN** the upload manager begins multipart upload for a staged file
- **THEN** it reads encrypted bytes from a sequential encryption stream instead of manually assembling multipart payloads from persisted plaintext chunks

#### Scenario: Encrypted stream preserves rclone format
- **WHEN** encrypted bytes are emitted for upload
- **THEN** the output begins with the rclone file header and continues with content encrypted using qrypt's existing block-compatible crypt format

### Requirement: Multipart upload MUST follow Quark-provided part size
The system MUST size multipart upload buffers from the part size returned by Quark upload preparation metadata instead of using a fixed locally configured part size.

#### Scenario: Server returns multipart size
- **WHEN** upload preparation returns metadata containing a multipart part size
- **THEN** the uploader reads encrypted bytes into part buffers using that server-provided size

### Requirement: Upload orchestration MUST keep qrypt-compatible multipart completion ordering
The system MUST orchestrate uploads in the order `UploadPre -> multipart upload -> UpdateHash verification -> Commit fallback -> Finish`, while sending encrypted content instead of plaintext content.

#### Scenario: Multipart upload required
- **WHEN** upload preparation succeeds and encrypted multipart data has been uploaded
- **THEN** the system sequentially uploads multipart parts, collects returned ETags, commits the multipart upload, and sends the finish request

#### Scenario: Multipart upload consumes encrypted stream
- **WHEN** a multipart part is uploaded
- **THEN** the uploaded bytes are read from the encryption stream rather than from a chunk-reassembly buffer

#### Scenario: Hash verification skips commit fallback
- **WHEN** encrypted multipart data has been uploaded and hash verification returns `finish=true`
- **THEN** the system finalizes file state without executing commit fallback
