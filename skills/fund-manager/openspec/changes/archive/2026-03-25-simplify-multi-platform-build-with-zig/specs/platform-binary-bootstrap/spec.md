## MODIFIED Requirements

### Requirement: Platform-aware binary bootstrap

The `fund-manager` skill SHALL detect the current operating system and architecture, resolve a supported target artifact from the 6-target matrix, and download a matching `fund-manager` core binary when no valid local binary is available.

#### Scenario: First run on supported platform and architecture

- **WHEN** a user invokes the `fund-manager` skill on a supported platform (Linux, macOS, Windows) and architecture (x86_64, aarch64) without a cached binary
- **THEN** the system MUST detect both OS and CPU architecture
- **THEN** the system resolves the matching target triple and downloads the binary package
- **THEN** the system installs the binary into a local cache path and executes it

#### Scenario: Unsupported platform or architecture

- **WHEN** a user invokes the skill on an unsupported OS or architecture combination
- **THEN** the system MUST fail fast with a clear error message listing all 6 supported targets
