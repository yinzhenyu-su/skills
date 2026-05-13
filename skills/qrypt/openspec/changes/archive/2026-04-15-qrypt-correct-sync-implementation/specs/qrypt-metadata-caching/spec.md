## MODIFIED Requirements

### Requirement: TTL-based Directory Metadata Caching
The system SHALL cache directory listings and file metadata for a configurable duration (default 60 seconds). Each node SHALL maintain a `baseServerMtime` and `baseServerSize` representing its state at the last successful synchronization. The system SHALL NOT overwrite local nodes marked as `isDirty` with remote metadata, but SHALL detect conflicts if the remote metadata has changed since the node's `baseServerMtime`.

#### Scenario: Subsequent listing with dirty node and remote change
- **WHEN** user lists a directory containing a file `/test.txt` which is `isDirty`
- **AND** the remote `updated_at` for `/test.txt` is greater than the node's `baseServerMtime`
- **THEN** the system SHALL mark the node as conflicted and trigger the conflict resolution protocol
- **AND** the listing returns the local `isDirty` node's metadata for the current path

#### Scenario: Subsequent listing
- **WHEN** user lists the same directory twice within the TTL
- **THEN** the second list operation returns data from the local cache without making a network request

## ADDED Requirements

### Requirement: On-Demand Metadata Refresh
The system SHALL verify the remote metadata of a file before starting an upload or a read operation if the cached metadata is older than the TTL.

#### Scenario: Upload with remote change detection
- **WHEN** an upload task for `/test.txt` starts
- **AND** a metadata refresh reveals that the remote file has been modified since `baseServerMtime`
- **THEN** the system SHALL abort the direct upload and initiate conflict resolution
