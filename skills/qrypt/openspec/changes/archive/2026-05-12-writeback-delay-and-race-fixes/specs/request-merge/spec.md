## ADDED Requirements

### Requirement: Batch delete files
The system SHALL support deleting multiple files in a single API request to Quark.

- The `ManageService` SHALL expose a `BatchDelete(fids []string) error` method
- The method SHALL use Quark's `/file/delete` endpoint with `filelist` containing all fids
- The `internal/fs/delete.go` rm path SHALL collect fids and call `BatchDelete` when deleting multiple files
- If the batch API fails (network error, partial success), the system SHALL fall back to deleting files individually

#### Scenario: Batch delete succeeds
- **WHEN** user runs `qrypt rm file1 file2 file3`
- **THEN** the system SHALL call `BatchDelete(["fid1", "fid2", "fid3"])` once
- **AND** no individual delete requests SHALL be made

#### Scenario: Batch delete partial failure
- **WHEN** `BatchDelete` returns a partial failure
- **THEN** the system SHALL fall back to individual delete for each remaining fid
- **AND** report which files failed to delete

### Requirement: ListFiles deduplication within cache TTL
The file service SHALL deduplicate concurrent `ListFiles(parentFid)` calls within the metadata cache TTL.

#### Scenario: Concurrent ListFiles deduped
- **WHEN** two goroutines call `ListFiles(parentFid)` simultaneously
- **THEN** only one API request SHALL be sent
- **AND** the second caller SHALL receive the cached result

### Requirement: expectedFid cleared on re-dirty
When a node transitions from clean (isDirty=false) to dirty (isDirty=true) due to a new Write, the `expectedFid` field SHALL be cleared to prevent `MergeRemoteChanges` from matching against a stale upload result.

#### Scenario: Write after upload clears expectedFid
- **WHEN** a file's upload completes successfully (expectedFid set)
- **AND** the user writes to the file again (isDirty=true)
- **THEN** `expectedFid` SHALL be set to `""`

### Requirement: Staging cleanup on startup
On startup, the system SHALL scan the staging directory and remove any `.staging` files whose fid does not correspond to an active node in the filesystem tree.

#### Scenario: Orphan staging file removed
- **WHEN** qrypt starts
- **AND** there exists `staging/<orphan_fid>.staging` where `<orphan_fid>` is not found in any node's `fid` or `localPath`
- **THEN** the file SHALL be deleted

#### Scenario: Active staging file preserved
- **WHEN** qrypt starts
- **AND** there exists `staging/<active_fid>.staging` where `<active_fid>` matches a node's fid
- **THEN** the file SHALL NOT be deleted
