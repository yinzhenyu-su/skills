## ADDED Requirements

### Requirement: TTL-based Directory Metadata Caching
The system SHALL cache directory listings and file metadata for a configurable duration (default 60 seconds) to reduce redundant network requests. The system SHALL NOT overwrite local nodes marked as `isDirty` with remote metadata during cache updates.

#### Scenario: Subsequent listing with dirty node
- **WHEN** user lists a directory containing a file `/test.txt` which is currently `isDirty` (e.g., uploading)
- **THEN** the listing returns the local `isDirty` node's metadata (e.g., local size) rather than the remote size
- **AND** the local `isDirty` node in the cache remains unchanged

#### Scenario: Subsequent listing
- **WHEN** user lists the same directory twice within the TTL
- **THEN** the second list operation returns data from the local cache without making a network request

### Requirement: Negative Caching
The system SHALL cache negative results (e.g., file not found) for a short duration to prevent repeated lookups for non-existent files.

#### Scenario: Lookup for missing file
- **WHEN** a request is made for a file that does not exist
- **THEN** the system caches the ENOENT result and returns it for subsequent requests within the negative TTL

### Requirement: Parallel Directory Listing
The system SHALL fetch directory pages in parallel for large directories to minimize the total listing time.

#### Scenario: Listing large directory
- **WHEN** a directory contains more than 100 files (1 page)
- **THEN** the system issues concurrent HTTP requests for all remaining pages
