## ADDED Requirements

### Requirement: Recursive Path Cache Update
When a directory is renamed or moved, the system SHALL recursively update the path keys in its in-memory node cache for all child files and directories.

#### Scenario: Subfolder rename
- **WHEN** directory `/A` containing `/A/b.txt` is renamed to `/X`
- **THEN** a lookup for `/X/b.txt` SHALL succeed by retrieving the updated node from the cache
