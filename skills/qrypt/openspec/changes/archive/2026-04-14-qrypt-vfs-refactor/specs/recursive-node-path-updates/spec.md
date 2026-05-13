## MODIFIED Requirements

### Requirement: Recursive Path Cache Update
When a directory is renamed or moved, the system SHALL recursively update the path keys in its in-memory node cache for all child files and directories, and SHALL rewrite any pending sync path metadata needed so queued or retried uploads can continue against the relocated tree.

#### Scenario: Subfolder rename
- **WHEN** directory `/A` containing `/A/b.txt` is renamed to `/X`
- **THEN** a lookup for `/X/b.txt` SHALL succeed by retrieving the updated node from the cache

#### Scenario: Rename with pending upload
- **WHEN** directory `/A` contains a dirty child file that has already been queued for background sync and the directory is renamed to `/X`
- **THEN** the system SHALL update the child path metadata so later retry, recovery, or completion logic converges on `/X/...` rather than the stale `/A/...` path
