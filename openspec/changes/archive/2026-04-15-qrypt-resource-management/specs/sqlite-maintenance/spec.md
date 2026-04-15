## ADDED Requirements

### Requirement: SQLite Database Optimization
The system SHALL periodically (at startup in background, or after large deletions) perform `VACUUM` and `PRAGMA incremental_vacuum` on the SQLite database to reclaim unused space, and run `PRAGMA optimize` to improve query planning.

#### Scenario: Database vacuum at startup
- **WHEN** the system starts up successfully
- **THEN** a background goroutine executes VACUUM and incremental_vacuum if the database has fragmented space

#### Scenario: Database vacuum after large deletion
- **WHEN** `EvictIfNeeded` deletes more than 100 chunks in a single run
- **THEN** a maintenance pass is triggered to reclaim space

### Requirement: Metadata Retention Policy
The system SHALL remove metadata for chunks that haven't been accessed for an extended period (30 days) and are not marked as dirty.

#### Scenario: Metadata cleanup
- **WHEN** the maintenance task runs
- **THEN** entries in the `chunks` table where `is_dirty = 0 AND access_time < datetime('now', '-30 days')` are deleted
