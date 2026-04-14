## 1. 测试基础设施搭建

- [ ] 1.1 创建 `skills/qrypt/internal/vfs/e2e_test.go` 文件结构
- [ ] 1.2 实现环境变量加载与校验逻辑（Cookie, Password, Paths）
- [ ] 1.3 实现 `qrypt` 驱动在后台协程的启动与健康检查逻辑
- [ ] 1.4 实现优雅卸载与进程清理逻辑（清理 `umount` 及残留文件）

## 2. 核心 CRUD 测试用例开发

- [ ] 2.1 实现 `TestE2E_DirectoryOperations`: 测试 `MkdirAll` 与 `RemoveAll`
- [ ] 2.2 实现 `TestE2E_FileLifecycle`: 测试 `WriteFile` -> `Wait Sync` -> `ReadFile` (校验 MD5)
- [ ] 2.3 实现 `TestE2E_RenameAndMove`: 测试文件与目录的重命名/移动一致性
- [ ] 2.4 实现 `TestE2E_Persistence`: 测试 `写入 -> 卸载 -> 重新挂载 -> 读取` 的持久化链路

## 3. macOS 特有行为与鲁棒性验证

- [ ] 3.1 实现 `TestE2E_MacOS_XAttr`: 测试扩展属性（xattr）的写入兼容性
- [ ] 3.2 实现 `TestE2E_ConcurrentStress`: 模拟并发小文件创建与写入压力
- [ ] 3.3 实现 `TestE2E_ErrorHandling`: 验证无效权限或网络异常下的错误反馈
- [ ] 3.4 完善测试日志重定向，确保失败时能输出驱动内部日志

## 4. 验证与归档

- [ ] 4.1 在本地 macOS 环境下运行所有 E2E 测试并记录结果
- [ ] 4.2 根据测试反馈优化 `QryptFS` 的错误处理逻辑（如针对 `Operation not permitted` 的特殊处理）
- [ ] 4.3 提交代码并归档变更
