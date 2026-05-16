# CLI UX Specification

## Purpose

Define POSIX-compatible flag conventions for CLI commands (ls, rm, mv) to provide consistent user experience and safety defaults.

## Requirements

### Requirement: POSIX Flags for ls Command
The `ls` command SHALL support POSIX-style flags for enhanced listing.

#### Scenario: Recursive listing
- **WHEN** user executes `qrypt ls -R <path>`
- **THEN** the system recursively lists all files and subdirectories.

#### Scenario: Human readable size
- **WHEN** user executes `qrypt ls -l -h <path>`
- **THEN** the system displays file sizes in human-readable formats (e.g., 1.5M, 2G).

#### Scenario: Sorting by time
- **WHEN** user executes `qrypt ls -t <path>`
- **THEN** the output is sorted by modification time, newest first.

#### Scenario: JSON output
- **WHEN** user executes `qrypt ls --json <path>`
- **THEN** the system outputs the listing metadata as a structured JSON array.

### Requirement: POSIX Flags for rm Command
The `rm` command SHALL require explicit flags for directory deletion and support safety overrides.

#### Scenario: Attempt to remove directory without flag
- **WHEN** user executes `qrypt rm <directory_path>`
- **THEN** the system aborts with an error indicating `-r` or `-R` is required.

#### Scenario: Force removal
- **WHEN** user executes `qrypt rm -f <non_existent_path>`
- **THEN** the system silently exits with success, ignoring the non-existent file error.

#### Scenario: Interactive removal
- **WHEN** user executes `qrypt rm -i <path>`
- **THEN** the system prompts for confirmation before sending the delete request.

### Requirement: POSIX Flags for mv Command
The `mv` command SHALL support safety flags to prevent accidental overwrites.

#### Scenario: Interactive move
- **WHEN** user executes `qrypt mv -i <src> <dst>` and `<dst>` exists
- **THEN** the system prompts for confirmation before overwriting `<dst>`.

#### Scenario: No-clobber move
- **WHEN** user executes `qrypt mv -n <src> <dst>` and `<dst>` exists
- **THEN** the system skips the operation without error and does not overwrite `<dst>`.
