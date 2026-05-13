## ADDED Requirements

### Requirement: Unified Zig-based build environment
The GitHub Actions system SHALL provide a unified build environment containing Rust, Zig, and `cargo-zigbuild` to enable cross-compilation from a single Linux host.

#### Scenario: Toolchain verification
- **WHEN** the GitHub Actions build job starts
- **THEN** it MUST verify that `rustc`, `zig`, and `cargo-zigbuild` are available in the PATH

### Requirement: Multi-architecture target mapping
The build system SHALL support a matrix of 6 targets covering major operating systems and architectures.

#### Scenario: Build matrix execution
- **WHEN** the GitHub Actions pipeline triggers a release build
- **THEN** it MUST execute builds for:
    1. x86_64-unknown-linux-gnu.2.17
    2. aarch64-unknown-linux-gnu.2.17
    3. x86_64-apple-darwin
    4. aarch64-apple-darwin
    5. x86_64-pc-windows-gnu
    6. aarch64-pc-windows-gnullvm
