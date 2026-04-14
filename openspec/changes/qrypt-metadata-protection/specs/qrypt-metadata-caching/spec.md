## MODIFIED Requirements

### Requirement: TTL-based Directory Metadata Caching
The system SHALL cache directory listings and file metadata for a configurable duration (default 60 seconds) to reduce redundant network requests. The system SHALL NOT overwrite local nodes marked as `isDirty` with remote metadata during cache updates.

#### Scenario: Subsequent listing with dirty node
- **WHEN** user lists a directory containing a file `/test.txt` which is currently `isDirty` (e.g., uploading)
- **THEN** the listing returns the local `isDirty` node's metadata (e.g., local size) rather than the remote size
- **AND** the local `isDirty` node in the cache remains unchanged
