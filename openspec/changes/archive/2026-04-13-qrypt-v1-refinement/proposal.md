## Why

Qrypt 项目已成功实现了夸克网盘的 rclone 兼容挂载原型，能够解密文件名并流式读取文件内容。为了将该原型提升为生产可用的工具，需要根据已验证的 rclone 算法逻辑更新技术规范，并引入性能优化（如本地分块缓存、并发预取）以及更完善的元数据管理。

## What Changes

- **巩固 rclone 兼容性**: 正式定义 NaCl SecretBox 和 EME-AES 在 Qrypt 中的实现标准。
- **引入高性能缓存 (Refinement)**: 启用基于 SQLite 的本地分块磁盘缓存，减少网盘 API 调用。
- **并发预取 (Prefetching)**: 实现读取时的后台分块预取逻辑，优化视频播放体验。
- **元数据持久化**: 将内存中的路径-FID 映射持久化到 SQLite，提升挂载后的首次加载速度。
- **稳定性修复**: 集成已发现的 401 错误处理、直链缓存和 Range 越界修复逻辑。

## Capabilities

### New Capabilities
- `metadata-persistence`: 将路径解析结果和文件属性存入本地数据库，避免重复的网盘 API 列表请求。
- `read-ahead-prefetcher`: 在顺序读取文件时自动触发后续分块的异步下载。
- `api-resilience`: 完善对夸克 API 频率限制（401/429）的自动重试和 Cookie 维护机制。

### Modified Capabilities
- `quark-driver`: 增加对直链缓存和 Cookie 自动更新的正式定义。
- `rclone-crypt`: 明确 EME-AES 和 NaCl 分块解密的实现细节及单元测试标准。
- `fuse-mount`: 完善文件日期 (Mtim) 和逻辑大小的映射规范。

## Impact

- **存储**: 需要在本地配置缓存路径，占用一定的磁盘空间。
- **安全性**: 缓存的分块数据可选是否加密存储。
- **体验**: 大文件打开速度和随机访问性能将显著提升。
