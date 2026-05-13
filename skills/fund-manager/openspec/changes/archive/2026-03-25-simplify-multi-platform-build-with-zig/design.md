## Context

The `fund-manager` skill currently uses a platform-specific build strategy in GitHub CI, requiring runners for Linux (x86_64), macOS (ARM64), and Windows (x86_64). This makes it difficult to add new architectures (like Linux ARM64 or macOS x86_64) without adding more infrastructure. `cargo-zigbuild` provides a way to cross-compile for all these targets from a single Linux environment.

## Goals / Non-Goals

**Goals:**
- Replace platform-specific build jobs with a single Linux-based build matrix.
- Support 6 targets: Linux (x86_64, aarch64), macOS (x86_64, aarch64), Windows (x86_64, aarch64).
- Ensure `rusqlite` (bundled) compiles correctly for all targets using `zig cc`.
- Simplify `bootstrap-fund-manager.sh` to handle architecture-aware downloads.

**Non-Goals:**
- Porting `fund-manager` code to other languages.
- Changing the distribution method (staying with GitHub Generic Package Registry).
- Supporting mobile platforms (iOS/Android).

## Decisions

### 1. Unified Target Matrix
We will use the following 6 LLVM-style target triples supported by `cargo-zigbuild`:
- `x86_64-unknown-linux-gnu.2.17` (Wide Linux compatibility)
- `aarch64-unknown-linux-gnu.2.17`
- `x86_64-apple-darwin`
- `aarch64-apple-darwin`
- `x86_64-pc-windows-gnu`
- `aarch64-pc-windows-gnullvm`

### 2. CI Tooling and Environment
- **Image**: Use `messense/cargo-zigbuild:latest` (or a specific version like `rust-1.85-zig-0.13`) as the base CI image. It comes pre-installed with `rust`, `zig`, and `cargo-zigbuild`.
- **macOS SDK Discovery**: The build script will dynamically resolve the `SDKROOT` using:
  `export SDKROOT=$(find /usr/osxcross -name "MacOSX*.sdk" | head -n 1)`
- **Cross-compilation for Rusqlite**: Since `rusqlite` is using the `bundled` feature, `cargo-zigbuild` will automatically use `zig cc` as the C compiler/linker for all targets, ensuring C code is cross-compiled correctly.

### 3. Bootstrap Script Logic
The `bootstrap-fund-manager.sh` will be updated to:
1. Detect OS: `uname -s` (Linux, Darwin, MINGW/MSYS/CYGWIN).
2. Detect Arch: `uname -m` (x86_64, aarch64/arm64).
3. Map to Target: A lookup table will map the (OS, Arch) pair to the correct target triple for downloading.

### 4. Windows Target Choice
We will switch from `pc-windows-msvc` to `pc-windows-gnu` (or `gnullvm` for arm64) to avoid needing MSVC on the Linux builder. Zig handles these targets natively and generates standalone executables.

## Risks / Trade-offs

- **[Risk] CI Build Duration** → **Mitigation**: Implement GitHub CI caching for `~/.cache/zig` and `target/` to avoid re-processing sysroots and intermediate objects.
- **[Risk] macOS SDK availability** → **Mitigation**: Use `messense/cargo-zigbuild` which includes pre-packaged SDK headers or provides a standard path for them.
- **[Risk] Glibc version mismatch** → **Mitigation**: Use the `.2.17` suffix in Zig targets to ensure compatibility with older Linux distributions.
- **[Risk] Windows GNU vs MSVC runtime** → **Mitigation**: `cargo-zigbuild` produces statically linked or minimal-dependency binaries for GNU targets that work on standard Windows installs.
installs.
