## ADDED Requirements

### Requirement: CLI Command Dispatch
The CLI tool SHALL parse user commands and dispatch them as JSON payloads to the background daemon's IPC server.

#### Scenario: Dispatching an emote command
- **WHEN** the user runs `yumi emote celebrate`
- **THEN** the CLI sends a JSON payload with the action "celebrate" to the local server

### Requirement: STDIN Piping Support
The CLI tool SHALL support reading text from standard input (stdin) to allow integration with shell pipelines.

#### Scenario: Piping error logs
- **WHEN** the user runs `cat error.log | yumi ask "What is wrong?"`
- **THEN** the CLI reads the piped content, appends the user prompt, and sends the payload to the daemon
