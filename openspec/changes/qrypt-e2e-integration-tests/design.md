## Context

`qrypt` 作为一个 FUSE 挂载工具，其核心难点在于与操作系统内核及 FUSE 层的交互。目前的测试无法覆盖真实挂载后的复杂行为（如 macOS 特有的文件创建序列）。为了解决这个问题，需要一套能够自动化模拟真实环境的集成测试。

## Goals / Non-Goals

**Goals:**
- 实现自动化的 FUSE 挂载与卸载流程。
- 通过标准系统调用（POSIX）验证全路径功能。
- 确认后台异步上传与本地缓存的一致性。
- 模拟并定位 macOS `Operation not permitted` 错误的根源。

**Non-Goals:**
- 不涉及具体的 UI 自动化测试。
- 不进行长期的压力或性能基准测试。
- 不提供跨平台的自动化测试环境配置（仅限开发/CI 环境手动触发）。

## Decisions

### 1. 挂载启动模式：In-Process vs Out-of-Process
- **Decision**: 在测试进程内启动后台协程运行 `fuse.NewFileSystemHost(fs).Mount()`。
- **Rationale**: 相比于启动外部二进制文件，进程内挂载允许测试代码直接访问 `QryptFS` 的内部状态（如 `isDirty` 标志），从而精确判断同步进度，而不需要猜测延迟时间。
- **Alternative**: 调用 `qrypt mount` 外部命令。缺点是难以观测驱动内部状态，且依赖二进制文件的构建。

### 2. 状态验证：内部状态轮询 vs 外部行为观察
- **Decision**: 结合使用。通过 `os.Stat` 验证外部行为，通过读取 `node.isDirty` 验证内部同步状态。
- **Rationale**: `isDirty` 是判断异步上传是否真正落地的唯一可靠信号。
- **Alternative**: 仅通过外部观察。这会导致测试不稳定，因为网络波动可能导致同步延迟不确定。

### 3. 环境隔离：临时测试路径
- **Decision**: 每次测试启动时在 `QRYPT_TEST_REMOTE_PATH` 下创建一个带有随机 UUID 的子目录。
- **Rationale**: 确保多次并行或残留测试数据互不干扰，保护真实网盘数据。

### 4. macOS 权限专项测试
- **Decision**: 显式包含对 `xattr`（扩展属性）的操作测试。
- **Rationale**: 许多 macOS 写入失败是因为 FUSE 未正确处理系统自动添加的扩展属性。

## Risks / Trade-offs

- **[Risk] 挂载点残留** → **Mitigation**: 使用 `defer` 确保 `Unmount` 和清理逻辑被执行；在测试开始前进行强力清理检查。
- **[Risk] 网络抖动导致测试失败** → **Mitigation**: 增加合理的重试次数和超时阈值。
- **[Risk] 操作系统权限限制 (CI)** → **Mitigation**: 在 CI 环境中需要授予 Runner 足够的权限（如 macOS 上的全磁盘访问权限或内核扩展加载权限）。
