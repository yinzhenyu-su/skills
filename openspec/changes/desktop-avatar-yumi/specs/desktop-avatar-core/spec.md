## ADDED Requirements

### Requirement: Background Daemon Execution
The application SHALL run as a background daemon without a standard OS window frame.

#### Scenario: Application Startup
- **WHEN** the user launches the application
- **THEN** it runs in the background and a tray icon is created

### Requirement: Transparent Click-Through Window
The main window SHALL be transparent and allow mouse events to pass through to underlying applications.

#### Scenario: Interacting with underlying apps
- **WHEN** the user clicks on the transparent area of the application window
- **THEN** the click event is passed to the application beneath it

### Requirement: Local IPC Server
The application SHALL start a local HTTP/WebSocket server on a known port to receive commands.

#### Scenario: Server Initialization
- **WHEN** the application starts
- **THEN** it listens on a designated local port for incoming JSON commands
