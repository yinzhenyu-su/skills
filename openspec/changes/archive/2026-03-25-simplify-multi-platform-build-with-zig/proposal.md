## Why

Currently, the `fund-manager` multi-platform build process relies on dedicated runners for Linux, macOS, and Windows. This setup is difficult to maintain, requires separate environment configurations for each OS, and limits the ability to easily support additional architectures (like ARM64) across all platforms.

## What Changes

- **Single Runner Build**: Consolidate the build process to a single Linux runner using `cargo-zigbuild`.
- **Expanded Target Support**: Add support for both x86_64 and arm64 architectures for Linux, macOS, and Windows (total 6 targets).
- **Toolchain Modernization**: Replace platform-specific build scripts with a unified `cargo-zigbuild` command that handles cross-compilation and C-dependency linking (via `zig cc`).
- **Bootstrap Update**: Update the `bootstrap-fund-manager.sh` script to detect architecture and map to the new 6-target matrix.

## Capabilities

### New Capabilities
- `zig-multi-platform-toolchain`: Defines the requirements for the unified build environment (Rust, Zig, cargo-zigbuild) and the specific target mappings.

### Modified Capabilities
- `gitlab-cross-platform-build-artifacts`: Update requirements to include 6 targets (x86_64 and arm64 for all platforms) and specify the use of a unified Linux runner.
- `platform-binary-bootstrap`: Update requirements to include architecture-aware binary detection and mapping.

## Impact

- `.github/workflows/release.yml`: Major simplification by removing macOS/Windows runners and using a build matrix on Linux.
- `skills/fund-manager/scripts/bootstrap-fund-manager.sh`: Updated platform/architecture detection logic.
- `skills/fund-manager/SKILL.md`: Updated artifact naming and platform support documentation.
- No changes to the core `fund-manager` Rust code are expected.
