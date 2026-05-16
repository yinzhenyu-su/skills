# Qrypt Project - Skills (Actuators & Procedures)

Organized based on Engineering Cybernetics principles.

## 1. System Actuators (Operations)
### 1.1 Implementing POSIX Commands
- **Pattern**: `resolve path` -> `validate target` -> `execute driver method` -> `report result`.
- **Constraint**: Always check if the driver implements the required interface (e.g., `drive.Writer`).

### 1.2 Path Subtree Update
- **Logic**: When renaming/moving a directory, use recursive top-down path updates to ensure child nodes stay reachable during the transition.
- **Fail-safe**: Perform memory index updates *after* successful API calls.

## 2. Stability & Error Correction (Disturbance Handling)
### 2.1 Atomic-like Downloads
- **Technique**: Use a `success` flag + `defer os.Remove(path)` to ensure partial or empty files are purged if the stream is interrupted.

### 2.2 Stdin Upload Spooling
- **Technique**: Buffer `stdin` to a temporary file to calculate exact size before initializing encryption. This avoids `PlainSize: -1` logic errors.

## 3. Performance Optimization (Efficiency Control)
### 3.1 Streaming Decryption
- **Logic**: Use `io.ReadCloser` streaming from the driver wrapped in a `DecryptingReader` instead of discrete 4KB block requests. This reduces IOPS and improves throughput.

### 3.2 Concurrent Transfer Pool
- **Structure**: Decouple directory scanning (recursive Walk) from the transfer execution (WorkerPool).
- **Feedback**: Monitor pool channel capacity to balance scanning and execution.
