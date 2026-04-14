## ADDED Requirements

### Requirement: SQLite Database Optimization
The system SHALL periodically (e.g., at startup or once a day) perform a `VACUUM` on the SQLite database to reclaim unused space.

#### Scenario: Database vacuum
- **WHEN** the system starts up
- **THEN** it executes a VACUUM command if the database file has significantly fragmented space

### Requirement: Metadata Retention Policy
The system SHALL remove metadata for files and chunks that haven't been accessed for an extended period (e.g., 30 days) if they are not marked as dirty.

#### Scenario: Metadata cleanup
- **WHEN** the maintenance task runs
- **THEN** entries in the `chunks` and `cached_names` tables older than the retention period are deleted
