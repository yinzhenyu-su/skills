## ADDED Requirements

### Requirement: Side-by-Side Conflict Resolution
When a conflict is detected between a local dirty node and its remote version, the system SHALL preserve the local changes by renaming the local node's path with a conflict suffix.

#### Scenario: Resolve conflict by side-by-side rename
- **WHEN** a conflict is detected for `/docs/report.txt`
- **THEN** the system SHALL rename the local dirty node's path to `/docs/report [Local Conflict].txt`
- **AND** the original path `/docs/report.txt` SHALL be updated with the latest remote metadata
- **AND** the new conflict node SHALL be queued for upload as a new file

### Requirement: Conflict Detection during Sync
The sync worker SHALL verify that the remote `updated_at` matches the node's `baseServerMtime` before committing an upload.

#### Scenario: Server-side change during local edit
- **WHEN** the sync worker begins processing `/memo.txt`
- **AND** `Remote.updated_at > node.baseServerMtime`
- **THEN** the system SHALL trigger the side-by-side rename and update the original path
