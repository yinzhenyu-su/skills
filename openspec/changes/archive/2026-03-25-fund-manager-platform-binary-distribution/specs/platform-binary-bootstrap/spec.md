## ADDED Requirements

### Requirement: Platform-aware binary bootstrap

The `fund-manager` skill SHALL detect the current operating system and architecture, resolve a supported target artifact, and download a matching `fund-manager` core binary when no valid local binary is available.

#### Scenario: First run on supported platform

- **WHEN** a user invokes the `fund-manager` skill on a supported platform without a cached binary
- **THEN** the system resolves the platform target and downloads the matching binary package
- **THEN** the system installs the binary into a local cache path and executes it

#### Scenario: Unsupported platform

- **WHEN** a user invokes the skill on an unsupported operating system or architecture
- **THEN** the system MUST fail fast with a clear error message listing supported targets

### Requirement: Binary cache reuse and recovery

The `fund-manager` bootstrap process SHALL reuse a valid cached binary and MUST recover from corrupted or incomplete local binaries.

#### Scenario: Cached binary is valid

- **WHEN** the local cached binary exists and passes integrity checks
- **THEN** the system MUST skip download and execute the cached binary directly

#### Scenario: Cached binary is corrupted

- **WHEN** integrity validation fails for the cached binary
- **THEN** the system MUST delete the invalid binary and re-download the same target artifact

### Requirement: Version and source override controls

The bootstrap process SHALL support environment-variable based controls for version pinning and download source overrides.

#### Scenario: Version pinning is set

- **WHEN** `FUND_MANAGER_VERSION` is provided
- **THEN** the system MUST request the binary artifact matching that version identifier

#### Scenario: Source URL override is set

- **WHEN** `FUND_MANAGER_CORE_URL` is provided
- **THEN** the system MUST use that URL as download source instead of the default release endpoint
