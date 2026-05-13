## MODIFIED Requirements

### Requirement: Three-platform CI build matrix

The GitHub Actions pipeline SHALL build `fund-manager` release binaries for Linux, macOS, and Windows (x86_64 and aarch64) using a unified Linux runner and `cargo-zigbuild`.

#### Scenario: Pipeline execution on main branch

- **WHEN** GitHub Actions runs for the default branch or a version tag
- **THEN** the pipeline MUST include build jobs for 6 target triples
- **THEN** each job MUST produce a release binary artifact for its target platform
- **THEN** all build jobs MUST execute on an Ubuntu-based runner environment
