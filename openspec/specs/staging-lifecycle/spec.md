## ADDED Requirements

### Requirement: Staging File Lifecycle Management
The system MUST track the lifecycle of staging files using a dedicated `staging_meta` table to prevent orphaned staging files from accumulating and wasting disk space.

#### Schema: staging_meta
- `fid TEXT PRIMARY KEY` — staging file identifier
- `local_path TEXT NOT NULL` — absolute path to the staging file
- `created_at DATETIME DEFAULT CURRENT_TIMESTAMP` — creation timestamp
- `updated_at DATETIME DEFAULT CURRENT_TIMESTAMP` — last modification timestamp
- `size INTEGER DEFAULT 0` — current file size in bytes
- `status TEXT DEFAULT 'active'` — one of: `active`, `syncing`, `abandoned`

#### Scenario: Staging file creation
- **WHEN** a new file upload begins and a staging file is created via `staging.Store.Create()`
- **THEN** a corresponding `staging_meta` entry is inserted with `status = 'active'`

#### Scenario: Staging file update
- **WHEN** data is written to the staging file via `staging.Store.WriteAt()` or `staging.Store.Truncate()`
- **THEN** the `staging_meta` entry is updated with the new `size` and `updated_at`

#### Scenario: Staging file deletion on successful sync
- **WHEN** a file upload completes successfully and the staging file is removed
- **THEN** the corresponding `staging_meta` entry is deleted via `RemoveStagingMeta()`

#### Scenario: Staging file orphaned (no pending node)
- **WHEN** the system starts up and finds a staging file on disk that has no corresponding `pending_node` entry
- **THEN** the orphaned staging file is deleted immediately

#### Scenario: Staging file abandoned
- **WHEN** a staging file's `pending_node` entry is cleaned up (e.g., user cancelled upload via Unlink) but the physical file remains
- **THEN** the `staging_meta` entry is marked as `status = 'abandoned'`

#### Scenario: Abandoned staging file cleanup
- **WHEN** `Maintenance()` runs and finds `staging_meta` entries with `status = 'abandoned'` and `updated_at` older than 24 hours
- **THEN** both the physical file and the `staging_meta` entry are deleted

### Requirement: Staging Metadata Sync on Write Operations
The staging store MUST update metadata (`SaveStagingMeta`, `UpdateStagingMeta`) whenever a staging file is created, written, or deleted to maintain accurate lifecycle tracking.

#### Scenario: Write operation records size
- **WHEN** `WriteAt()` completes successfully and wrote `N` bytes
- **THEN** `UpdateStagingMeta(fid, newSize)` is called with the current file size from `f.Stat()`

### Requirement: Startup Orphan Cleanup
The system MUST clean up orphaned staging files at startup before attempting to recover pending uploads.

#### Scenario: Startup orphan cleanup
- **WHEN** `NewCacheManager()` initializes
- **THEN** `cleanupOrphanedStagingFiles()` is called to remove staging files that have no corresponding `pending_node`
