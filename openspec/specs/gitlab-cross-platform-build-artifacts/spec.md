## ADDED Requirements

### Requirement: Three-platform CI build matrix

The CI pipeline SHALL build `fund-manager` release binaries for Linux, macOS, and Windows using platform-appropriate runners.

#### Scenario: Pipeline execution on main branch

- **WHEN** CI runs for the default branch
- **THEN** the pipeline MUST include build jobs for Linux, macOS, and Windows targets
- **THEN** each job MUST produce a release binary artifact for its target platform

### Requirement: Stable artifact naming convention

The CI pipeline SHALL publish artifacts with deterministic names that include version and Rust target triple.

#### Scenario: Artifact naming during build

- **WHEN** a platform build job packages output
- **THEN** the artifact filename MUST follow the pattern `fund-manager-v<version>-<target-triple>.<archive-ext>`
- **THEN** Windows artifacts MUST contain `.exe` inside the package

### Requirement: Downloadable release channel

The CI process SHALL expose built artifacts through a stable GitLab download channel suitable for skill bootstrap consumption.

#### Scenario: Tagged release publish

- **WHEN** CI is triggered by a version tag
- **THEN** the pipeline MUST publish platform artifacts to a long-lived endpoint (Package Registry or Release assets)
- **THEN** bootstrap clients MUST be able to retrieve artifacts without relying on expiring job artifact links

### Requirement: Documentation alignment for runtime bootstrap

Skill documentation SHALL describe runtime dependencies and required environment variables for downloading private artifacts.

#### Scenario: Private repository setup

- **WHEN** artifact source requires authentication
- **THEN** documentation MUST specify required variables for project identification and token-based access
- **THEN** documentation MUST include a troubleshooting path for HTTP 401 and 403 responses
