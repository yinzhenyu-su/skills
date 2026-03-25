## 1. Build Environment Setup

- [x] 1.1 Verify presence of Rust, Zig, and `cargo-zigbuild` in the local development environment
- [x] 1.2 Identify/prepare a Docker image containing the necessary toolchains and macOS SDK headers

## 2. GitHub Actions Configuration

- [x] 2.1 Create `.github/workflows/release.yml` and configure the build matrix
- [x] 2.2 Implement the build matrix for 6 targets using `strategy: matrix`:
    - `x86_64-unknown-linux-gnu.2.17`
    - `aarch64-unknown-linux-gnu.2.17`
    - `x86_64-apple-darwin`
    - `aarch64-apple-darwin`
    - `x86_64-pc-windows-gnu`
    - `aarch64-pc-windows-gnullvm`
- [x] 2.3 Update the `cargo-zigbuild` command in GitHub Actions to use the correct target and release flags
- [x] 2.4 Configure GitHub Actions caching for `~/.cache/zig` and `target/`
- [x] 2.5 Ensure artifact packaging (tar.gz/zip) correctly handles the 6 target naming convention and upload to GitHub Release
- [x] 2.6 (Optional) Remove or disable `.gitlab-ci.yml`

## 3. Bootstrap Script Update

- [x] 3.1 Update `skills/fund-manager/scripts/bootstrap-fund-manager.sh` to detect CPU architecture using `uname -m`
- [x] 3.2 Update the OS detection logic to be more robust across Linux, macOS, and Windows environments
- [x] 3.3 Implement the 6-target lookup table mapping (OS, Arch) to the correct target triple
- [x] 3.4 Test the bootstrap script on at least one alternative architecture (e.g., Linux ARM64 if available)

## 4. Documentation and Metadata

- [x] 4.1 Update `skills/fund-manager/SKILL.md` to reflect the 6-platform support matrix
- [x] 4.2 Verify and update any environment variable descriptions related to binary downloads
- [x] 4.3 Update the GitHub Actions 产物规范 (Artifact Specification) section in `SKILL.md`
