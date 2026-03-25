## Context

The goal is to build a desktop AI avatar (YUMI) controlled via a Command Line Interface (CLI). This requires two decoupled but communicating systems: a desktop graphical interface to render the avatar and play audio, and a CLI tool to act as the user's primary interaction method. The project aims to provide an engaging, responsive, and low-latency interaction model for developers.

## Goals / Non-Goals

**Goals:**
- Provide a responsive 3D avatar rendering experience on the desktop using Tauri and Three.js.
- Ensure the desktop application window is transparent, frameless, and allows click-through (except when interacting with the avatar directly, if implemented).
- Establish a reliable local Inter-Process Communication (IPC) mechanism between the CLI and the Tauri daemon.
- Implement an audio-amplitude-based lip-sync mechanism for the avatar.
- Support state machine-driven animations triggered by CLI commands.

**Non-Goals:**
- Multi-avatar support; initially hardcoded to the YUMI character.
- Complex physics interactions (e.g., cloth simulation beyond VRM SpringBone defaults).
- Deployment as a general-purpose VTubing software.
- Deep system integration beyond the immediate CLI workflows.

## Decisions

- **Host Framework: Tauri (Rust + Webview)**
  - *Rationale*: Tauri provides a very lightweight footprint compared to Electron, making it ideal for a long-running background daemon. It natively supports transparent and frameless windows. The Rust backend is performant and integrates well with the planned Rust-based CLI.
  - *Alternatives Considered*: Godot (better visual capabilities, but harder to integrate with local web technologies and UI overlays); Electron (heavy resource usage).
- **3D Rendering: Three.js + `@pixiv/three-vrm`**
  - *Rationale*: VRM is the industry standard for 3D anime avatars. `@pixiv/three-vrm` is robust, officially maintained, and supports blendshapes (expressions) and SpringBone (physics) out of the box.
- **Communication Protocol: Local HTTP / WebSocket Server**
  - *Rationale*: The Tauri Rust backend will spin up a local server (e.g., WebSocket) on a specific port or via a Unix Domain Socket (UDS). The CLI acts as a client sending JSON payloads. This ensures immediate, low-latency execution of commands like `yumi emote celebrate`.
  - *Alternatives Considered*: Writing to a local file (slower, polling required).
- **Audio & Lip-Sync: Web Audio API `AnalyserNode`**
  - *Rationale*: Real-time lip-sync via AI timestamp generation is complex and error-prone. A simpler, effective "pseudo lip-sync" can be achieved in the frontend by mapping the volume amplitude of the playing audio buffer to the VRM's `aa`, `ih`, `ou`, `ee`, `oh` blendshapes.

## Risks / Trade-offs

- **[Risk] Window Management Quirks across OS** → *Mitigation*: Tauri's support for `transparent: true`, `decorations: false`, and `set_ignore_cursor_events` varies slightly between macOS and Windows. We will prioritize macOS development first (user's OS is darwin) and test thoroughly.
- **[Risk] Animation Retargeting from Mixamo to VRM** → *Mitigation*: Use established tools (Blender plugins or frontend libraries like `vrm-mixamo`) to map Mixamo's `.fbx` skeleton to the VRM skeleton correctly.
- **[Risk] Background Resource Usage** → *Mitigation*: Ensure the Three.js render loop is optimized (e.g., capping FPS, pausing rendering when not visible or inactive) to minimize CPU/GPU load while running as a background task.
