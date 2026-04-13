## Context

本项目旨在构建一个名为 `qrypt` 的 Go 语言命令行工具，用于将已经由 rclone 加密并上传到夸克网盘的特定目录透明挂载为 macOS 本地磁盘。核心挑战在于对齐 rclone 的 EME-AES 文件名加密和 NaCl SecretBox 分块内容加密逻辑。

## Goals / Non-Goals

**Goals:**
- **rclone 100% 兼容**: 读取由 rclone 生成的加密数据。
- **子目录挂载**: 通过参数 `--root-path "/MyData/Prv"` 指定挂载点。
- **透明加解密**: EME-AES 文件名解密 + Salsa20/Poly1305 分块解密。
- 实现基于 LRU 策略的本地分块磁盘缓存。
- 优化 macOS Finder 性能。

**Non-Goals:**
- 不支持修改 rclone 的加密配置（第一版仅支持 rclone 的标准默认配置）。
- 第一阶段不处理加密文件的修改上传（主要针对大文件读取优化）。

## Decisions

- **加解密核心**: 使用 `golang.org/x/crypto/nacl/secretbox`。
- **文件名解密**: 使用 rclone 风格的 `AES-256-EME` 结合自定义字符集的 `Base32` 编码。
- **路径解析器**: 在挂载前从根目录递归查找指定的子路径，获取其 `fid` 并作为 VFS 根。
- **密钥派生**: 复刻 rclone 的 `scrypt` 逻辑，支持主密码 (`password`) 和盐值 (`salt`)。
- **缓存实现**: 采用分块缓存（64KB + 16B），本地文件名记录为网盘原始加密名的 Hash。

## Risks / Trade-offs

- **[Risk] 性能开销** → **[Mitigation]** 缓存解密后的文件名映射，避免频繁调用 scrypt/EME。
- **[Risk] 兼容性失败** → **[Mitigation]** 编写严格的单元测试，模拟 rclone 的加密输出，确保解密结果完全一致。
- **[Trade-off] 只读模式** → 为了确保大文件性能和对齐 rclone，初期优先实现高并发下载和解密。

