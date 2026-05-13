## ADDED Requirements

### Requirement: Delete operation MUST converge local and remote state

The system MUST treat successful remote deletion as the trigger to converge local state, including node index, pending upload records, dirty chunks, and in-memory cache entries.

#### Scenario: Delete file successfully

- **WHEN** a file delete request succeeds on remote API
- **THEN** the system MUST remove the file node from VFS and clear related pending/dirty/cache state for that path or fid

#### Scenario: Delete directory successfully

- **WHEN** a directory delete request succeeds on remote API
- **THEN** the system MUST remove the directory node and clear related local pending state under the deleted subtree

### Requirement: Failed delete MUST preserve recoverable local state

If remote deletion fails, the system MUST preserve local state required for retry and MUST NOT mark the object as deleted locally.

#### Scenario: Remote delete fails

- **WHEN** remote delete API returns an error
- **THEN** the system MUST keep node and pending metadata unchanged and return an error to the caller

### Requirement: Recovery MUST skip invalid pending delete-related records

During mount recovery, the system MUST detect invalid or orphan pending records related to deleted/nonexistent nodes and MUST clean them up safely.

#### Scenario: Orphan pending record detected

- **WHEN** a pending record references a missing node or invalid metadata
- **THEN** the system MUST skip retry for that record and remove it from local pending storage
