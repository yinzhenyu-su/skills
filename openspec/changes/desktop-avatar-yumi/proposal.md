## Why

Developers spend a significant amount of time in terminals and code editors. Traditional terminal outputs (like script execution results and error logs) and system-level notifications, while efficient, lack warmth and interactive engagement. With the advancement of Large Language Models (LLMs) and high-quality Text-to-Speech (TTS) technologies, creating an AI assistant with a tangible "physical" presence on the desktop is now possible. This project aims to transform the mundane development process into a more engaging experience by providing emotional feedback, code assistance, and voice interaction through a virtual avatar named YUMI.

## What Changes

- **Desktop Host Application**: Create a lightweight Tauri-based desktop application that runs as a daemon/background process with a transparent, frameless, and click-through window.
- **3D Model Rendering**: Implement a WebGL renderer using Three.js and `@pixiv/three-vrm` within the Tauri webview to display a 3D anime-style avatar (VRM format).
- **Animation and Expression Control**: Support state-machine based animations (e.g., Mixamo `.glb` files for idle, celebrate, think) and dynamic facial expressions via VRM BlendShapes.
- **Audio and Lip-Sync**: Integrate TTS for voice responses and implement real-time audio amplitude analysis (Web Audio API) for pseudo lip-syncing.
- **Command Line Interface (CLI)**: Develop a standalone CLI tool (e.g., `yumi`) built in Rust to send text, commands, and prompts to the desktop daemon via local IPC (WebSocket or HTTP).
- **AI Integration**: Connect the backend to an LLM provider to process developer queries and generate spoken responses alongside appropriate animations.

## Capabilities

### New Capabilities
- `desktop-avatar-core`: The Tauri application lifecycle, tray management, transparent window setup, and local IPC server (HTTP/WebSocket).
- `avatar-rendering`: The Three.js integration, VRM model loading, lighting, and camera setup in the frontend webview.
- `avatar-animation`: The animation system handling Mixamo bone animations, VRM blendshapes (expressions), and audio-amplitude-based lip-sync.
- `avatar-cli`: The Rust-based CLI client (`yumi`) for dispatching commands, text, and queries to the running daemon.
- `ai-interaction`: The integration with LLMs (e.g., OpenAI) for query processing and TTS engines (e.g., edge-tts, xiaomi-tts) for voice generation.

### Modified Capabilities
- (None)

## Impact

This is a greenfield project that will introduce a new standalone desktop application and a companion CLI tool to the user's workspace. It does not directly modify existing systems but provides a new interface for interacting with development workflows and AI assistants. It will rely on local network ports for IPC and require audio playback permissions.
