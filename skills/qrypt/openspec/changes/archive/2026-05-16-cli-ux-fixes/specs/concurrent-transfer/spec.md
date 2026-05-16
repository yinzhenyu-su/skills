## MODIFIED Requirements

### Requirement: Incremental Transfer Updates
The transfer commands SHALL support an update flag to skip unmodified files by comparing file sizes.

#### Scenario: Push with update flag
- **WHEN** user executes `qrypt push -u <local_dir> <remote_path>`
- **THEN** the system compares the local file size with the decrypted size of the remote file, and only uploads files that do not exist or differ in size.

#### Scenario: Pull with update flag
- **WHEN** user executes `qrypt pull -u <remote_dir> <local_path>`
- **THEN** the system compares the decrypted size of the remote file with the local file size, and only downloads files that do not exist or differ in size.

## ADDED Requirements

### Requirement: Efficient Downloading
The `Downloader` SHALL stream file bodies sequentially rather than issuing discrete requests for individual encryption blocks.

#### Scenario: Download large file
- **WHEN** a file is downloaded
- **THEN** the system issues exactly two read requests to the driver: one for the header, and one continuous stream for the file body.
