## ADDED Requirements

### Requirement: CLI discovers running daemon via socket scan

CLI commands SHALL discover the running qrypt daemon by checking the Unix socket at `~/.qrypt/qryptd.sock`. The discovery SHALL happen automatically before every CLI operation that requires daemon access.

#### Scenario: Daemon socket exists
- **WHEN** `~/.qrypt/qryptd.sock` exists and is listening
- **THEN** CLI SHALL connect to the socket and proceed with the requested operation

#### Scenario: Daemon socket does not exist
- **WHEN** `~/.qrypt/qryptd.sock` does not exist or is not listening
- **THEN** CLI SHALL NOT display an error; instead SHALL trigger auto-daemon startup

#### Scenario: Socket path override via env
- **WHEN** env `QRYPTD_SOCKET` is set
- **THEN** CLI SHALL use that path instead of the default `~/.qrypt/qryptd.sock`

#### Scenario: Multiple mounts in one daemon
- **WHEN** a single daemon manages multiple mount instances (configured via `[[mounts]]`)
- **THEN** CLI SHALL use `mount_name:path` syntax to select the target mount, sending the mount name as part of the RPC params; the daemon SHALL route to the correct `MountInstance` internally
