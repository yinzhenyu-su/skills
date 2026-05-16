## ADDED Requirements

### Requirement: Recursive Directory Transfers
The `push` and `pull` commands SHALL default to recursively processing directories if a directory path is provided.

#### Scenario: Push a local directory
- **WHEN** user executes `qrypt push <local_dir> <remote_path>`
- **THEN** the system traverses `<local_dir>` and uploads all contained files, mirroring the local structure at `<remote_path>`.

#### Scenario: Pull a remote directory
- **WHEN** user executes `qrypt pull <remote_dir> <local_path>`
- **THEN** the system traverses `<remote_dir>` and downloads all contained files, mirroring the structure at `<local_path>`.

### Requirement: Concurrent Transfer Execution
The `push` and `pull` commands SHALL support executing multiple transfers simultaneously using a worker pool.

#### Scenario: Specify concurrency level
- **WHEN** user executes `qrypt push --transfers 4 <local_dir> <remote_path>`
- **THEN** the system spawns 4 worker routines to process the upload queue concurrently.

### Requirement: Incremental Transfer Updates
The transfer commands SHALL support an update flag to skip unmodified files.

#### Scenario: Push with update flag
- **WHEN** user executes `qrypt push -u <local_dir> <remote_path>`
- **THEN** the system only uploads files that are newer or differ in size from the corresponding file in `<remote_path>`.

### Requirement: Dry Run Transfer Mode
The transfer commands SHALL support a preview mode that performs no actual data movement.

#### Scenario: Dry run push
- **WHEN** user executes `qrypt push --dry-run <local_dir> <remote_path>`
- **THEN** the system logs all files that would be uploaded but does not execute the actual upload API calls.
