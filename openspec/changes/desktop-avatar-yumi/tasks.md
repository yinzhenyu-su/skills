## 1. Project Initialization

- [ ] 1.1 Scaffold Tauri project structure using `create-tauri-app`
- [ ] 1.2 Initialize the standalone Rust CLI tool (`yumi`) within the workspace
- [ ] 1.3 Configure Tauri for a frameless, transparent, and click-through window (adjusting `tauri.conf.json`)

## 2. Desktop Render & 3D Avatar (MVP 1)

- [ ] 2.1 Set up Three.js in the Tauri frontend
- [ ] 2.2 Integrate `@pixiv/three-vrm` and load a sample VRM model (YUMI)
- [ ] 2.3 Configure lighting and camera for an anime-style rendering
- [ ] 2.4 Verify alpha channel setup to ensure the 3D model background blends perfectly with the desktop

## 3. IPC Communication (MVP 2)

- [ ] 3.1 Implement a local WebSocket or HTTP server within the Tauri Rust backend
- [ ] 3.2 Define JSON payload schemas for commands (e.g., `say`, `emote`, `ask`)
- [ ] 3.3 Implement the CLI parser using `clap` to read arguments and stdin
- [ ] 3.4 Establish the connection from CLI to the Tauri server and send test payloads
- [ ] 3.5 Pass the received payload from Tauri Rust backend to the frontend webview via Tauri events

## 4. Animation and Lip-sync

- [ ] 4.1 Implement VRM BlendShape toggling in the frontend based on received `emote` commands
- [ ] 4.2 Set up the Web Audio API context and `AnalyserNode`
- [ ] 4.3 Map audio volume amplitude from the `AnalyserNode` to the VRM mouth BlendShapes (pseudo lip-sync)
- [ ] 4.4 Integrate Mixamo `.glb` animations and blend them over the default VRM T-pose/idle state

## 5. AI and Audio Integration (MVP 3)

- [ ] 5.1 Integrate an LLM provider (e.g., OpenAI API) in the Tauri backend for the `ask` command
- [ ] 5.2 Integrate a Text-to-Speech (TTS) engine to generate audio buffers from text strings
- [ ] 5.3 Implement the pipeline: Receive `ask` -> Fetch LLM response -> Fetch TTS audio -> Play audio + lip-sync in frontend -> Output text in CLI or UI bubble
