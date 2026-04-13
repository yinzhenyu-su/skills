## Context

Qrypt 的 MVP (Minimum Viable Product) 已经证明了基于 Go 的 rclone 兼容挂载方案在 macOS 上是可行的。然而，目前的实现存在 API 调用过频（未充分缓存直链）、响应延迟高（无本地分块缓存）以及元数据在重启后丢失等问题。

## Goals / Non-Goals

**Goals:**
- 将 MVP 中的临时逻辑正式化为稳定的模块结构。
- 实现分块磁盘缓存，支持 LRU 清理。
- 实现基于预取 (Read-ahead) 的下载优化。
- 完善元数据管理，支持重启后快速挂载。

**Non-Goals:**
- 本阶段仍不处理文件的写入和修改加密。
- 不支持 rclone 的非标准加密配置（如非 64KB 分块）。

## Decisions

- **缓存架构**: 采用“双层缓存”模式。
    - **元数据层**: SQLite 存储 `Path -> FID`、`FID -> Meta (size, mtime, nonce)`。
    - **数据块层**: 本地磁盘存储解密后的 `64KB` 块，文件名以 `fid_index` 命名。
- **预取算法**: 读取当前块 `i` 时，检查缓存中是否存在 `i+1` 到 `i+N` 块。若不存在，启动异步工作协程并行下载。
- **错误恢复**: 当遇到 `401` 或 `429` 错误时，系统应自动刷新直链或等待重试，而不是直接向 VFS 返回错误导致 Finder 挂起。
- **代码重构**: 将 `internal/vfs` 中的复杂解密逻辑下沉到 `internal/cache` 和 `internal/crypt` 中，保持 VFS 层简洁。

## Risks / Trade-offs

- **[Risk] 磁盘空间占用** → **[Mitigation]** 设置严格的高低水位线（如默认 10GB 上限），由后台协程定期清理。
- **[Risk] 缓存一致性** → **[Trade-off]** 初期假设网盘数据不会被第三方修改（或仅通过 Qrypt 访问），若发生外部修改，用户需手动清除缓存或等待 TTL 到期。
- **[Trade-off] 性能 vs 实时性** → 优先保证视频播放的流畅度，元数据缓存的 TTL 设为 30 分钟。
