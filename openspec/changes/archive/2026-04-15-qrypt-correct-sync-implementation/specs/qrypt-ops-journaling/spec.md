## ADDED Requirements

### Requirement: Operations Logging (Journaling)
The system SHALL record metadata operations (Rename, Delete, Mkdir) in an `ops_log` table in the local cache before attempting the remote API call.

#### Scenario: Metadata operation journaling
- **WHEN** user renames `/old.txt` to `/new.txt`
- **THEN** the system SHALL insert a `PENDING` record in `ops_log`
- **AND** the system SHALL update its in-memory VFS state immediately
- **AND** only then SHALL the system attempt the remote API call

### Requirement: Recovery of Pending Operations
Upon startup, the system SHALL scan the `ops_log` for any operations marked as `PENDING` and attempt to retry them.

#### Scenario: Recovery after crash
- **WHEN** the system starts and finds a `PENDING` rename in `ops_log`
- **THEN** it SHALL attempt to execute the rename on the server
- **AND** once successful, mark the log entry as `DONE`
