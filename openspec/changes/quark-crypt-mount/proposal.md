## Why

## Why

现有网盘（如夸克网盘）缺乏原生的透明加密功能，导致用户隐私数据在云端处于明文状态。用户通常使用 rclone 进行加密挂载，但在 macOS 下其性能（尤其是 FUSE 适配和缓存）有待优化。本项目旨在提供一个专为 macOS 优化、完全兼容 rclone 加密协议、支持挂载特定网盘目录并具备高效分块缓存的工具。

## What Changes

- 开发全新的 Go 语言命令行工具 `qrypt`。
- **rclone 兼容性 (CORE)**: 实现与 rclone `crypt` 后端完全一致的加解密算法（NaCl SecretBox 用于内容，EME-AES 用于文件名）。
- **特定路径挂载**: 支持通过路径（如 `/Movies/Private`）定位网盘目录并将其作为挂载根节点。
- 实现夸克网盘 API 驱动，支持分块读写和断点续传。
- 实现基于 LRU 策略的本地分块缓存系统（Scheme B），提升二次访问速度。
- 集成 macFUSE，优化 macOS Finder 元数据请求拦截。

## Capabilities

### New Capabilities
- `quark-driver`: 负责 API 交互，新增 `PathResolver` 用于将路径解析为夸克 `fid`。
- `rclone-crypt`: **替代原 chunk-crypt**。实现 rclone 兼容的 scrypt 密钥派生、EME-AES 文件名加解密和 NaCl 分块内容加解密。
- `lru-cache`: 管理本地分块缓存，增加文件名映射缓存以加速 `ls` 操作。
- `fuse-mount`: 处理 macFUSE 挂载，映射 rclone 风格的逻辑文件大小。

## Impact

- **安全性**: 用户需提供与 rclone 一致的 `password` 和 `salt`（可选）。
- **性能**: 文件名解密会增加 `ls` 的 CPU 开销，需通过缓存优化。
- **存储**: 加密后的文件结构与 rclone 互通，用户可随时切回 rclone。

- **存储**: 本地需要预留一定的磁盘空间用于 LRU 缓存。
