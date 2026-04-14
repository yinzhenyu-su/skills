## Why

目前 `qrypt` 的测试主要集中在单元逻辑，缺乏真实的端到端（E2E）挂载测试。在 macOS 环境下，用户反馈频繁出现 `Operation not permitted` 错误，这类问题通常涉及内核、FUSE 驱动与操作系统权限的复杂交互，仅靠 Mock 测试无法定位和复现。通过建立一套由外部环境变量驱动的真实 CRUD 测试，可以直接在挂载点执行 POSIX 系统调用，从而闭环验证驱动的稳定性、权限兼容性以及异步同步的可靠性。

## What Changes

- **新增集成测试**: 创建 `skills/qrypt/internal/vfs/e2e_test.go`，实现完整的文件系统生命周期测试。
- **环境变量驱动**: 测试将完全依赖外部提供的 `QRYPT_TEST_COOKIE`, `QRYPT_TEST_PASSWORD`, `QRYPT_TEST_MOUNT_POINT`, `QRYPT_TEST_REMOTE_PATH` 等变量。
- **挂载管理**: 实现自动化的后台驱动启动、健康检查（就绪等待）以及优雅卸载逻辑。
- **多场景覆盖**: 包含大/小文件写入、并发操作、重命名、异步同步状态校验以及 macOS 特有的元数据/扩展属性测试。

## Capabilities

### New Capabilities
- `e2e-integration-testing`: 定义如何通过外部环境配置进行端到端的挂载验证流程。

### Modified Capabilities
- `qrypt-delete-consistency`: 增加在真实挂载环境下的删除一致性要求。
- `smart-settlement-sync`: 完善异步同步在真实系统调用下的确定性反馈要求。

## Impact

- **测试基础设施**: 引入了对真实网盘环境和本地 FUSE 挂载点的依赖。
- **CI/CD**: 需要在 CI 环境中配置相应的 Secret 才能运行此类测试。
- **代码结构**: 可能会为了更好地支持 E2E 测试而暴露部分内部状态（如同步完成的信号）。
