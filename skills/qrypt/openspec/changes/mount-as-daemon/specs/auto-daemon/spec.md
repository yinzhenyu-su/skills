## ADDED Requirements

### Requirement: CLI auto-starts headless daemon when none running

When a CLI command (push, pull, ls, cat, rm, mv, mkdir) detects no running daemon, it SHALL automatically start a headless daemon process in the background. The headless daemon SHALL include WS server, SessionManager, MountManager, and TransferOrchestrator, but no FUSE mount.

#### Scenario: Auto-start on first CLI command
- **WHEN** user runs `qrypt ls /remote/folder` and no daemon socket exists
- **THEN** CLI SHALL start a headless daemon (`qrypt mount --daemon`) in background, wait for socket to appear, connect, and execute the command

#### Scenario: Auto-start timeout
- **WHEN** headless daemon fails to create socket within 500ms
- **THEN** CLI SHALL exit with error "无法启动 daemon"

#### Scenario: Consecutive CLI commands reuse same daemon
- **WHEN** user runs `qrypt ls; qrypt cat /file` in sequence
- **THEN** the second command SHALL reuse the daemon started by the first command (socket already exists)

#### Scenario: Other CLI commands find running daemon
- **WHEN** user runs `qrypt push file.mp4 /remote/` with an existing daemon from a previous mount
- **THEN** CLI SHALL connect to the existing socket and execute the command through the running daemon

### Requirement: Headless daemon auto-exits on idle

The headless daemon SHALL automatically exit after an idle timeout when no WebSocket clients are connected.

#### Scenario: Idle timeout
- **WHEN** headless daemon has zero WS connections for 30 seconds
- **THEN** daemon SHALL shutdown gracefully and remove the socket file

#### Scenario: Daemon with active mount
- **WHEN** daemon is running with a FUSE mount (not headless)
- **THEN** idle timeout SHALL NOT apply; daemon stays alive as long as FUSE is mounted

#### Scenario: Explicit shutdown via RPC
- **WHEN** user runs `qrypt mount --stop-daemon` or sends shutdown RPC
- **THEN** daemon SHALL shutdown immediately regardless of idle state
