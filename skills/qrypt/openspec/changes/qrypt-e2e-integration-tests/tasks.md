## 1. 测试基础设施搭建

- [x] 1.1 创建 `internal/fs/e2e_test.go` 文件结构（756 行，39 个测试函数）
- [ ] 1.2 实现环境变量加载与校验逻辑（Cookie, Password, Paths）— 当前使用 `newTestFS` 内建 Mock HTTP Server，无真实网盘环境变量
- [ ] 1.3 实现 `qrypt` 驱动在后台协程的启动与健康检查逻辑 — 当前使用 `newTestFS` 进程内直调，非独立 mount 进程
- [ ] 1.4 实现优雅卸载与进程清理逻辑 — 当前每个测试独立创建临时目录，通过 `t.TempDir()` 自动清理

## 2. 核心 CRUD 测试用例开发（已实现，以下为当前覆盖）

- [x] 2.1 目录操作: `TestE2E_DirectoryCreateAndList`, `TestE2E_DirectoryWithFiles`, `TestE2E_DeepNestedDirectory`
- [x] 2.2 文件生命周期: `TestE2E_FileCreateWriteRead`, `TestE2E_FileMultipleWrites`, `TestE2E_FileOverwriteMidRegion`, `TestE2E_EmptyFile`, `TestE2E_Flush`, `TestE2E_Fsync`, `TestE2E_Truncate`
- [x] 2.3 重命名: `TestE2E_Rename`, `TestE2E_RenameDir`
- [ ] 2.4 持久化测试（写入 -> 卸载 -> 重新挂载 -> 读取）— 当前为进程内 mock，缺乏真实的卸载重挂载验证

## 3. 扩展测试覆盖（已实现，以下为当前覆盖）

- [x] 3.1 xattr: `TestE2E_XattrOperations` — 扩展属性读写测试
- [x] 3.2 并发: `TestE2E_ConcurrentCreateDifferentFiles`, `TestE2E_ConcurrentReadsSameFile`, `TestE2E_ConcurrentReadsDifferentOffsets`
- [x] 3.3 错误处理: `TestE2E_OpenNonExistent`, `TestE2E_UnlinkNonExistent`, `TestE2E_ReadPastEOF`, `TestE2E_ReadBeyondFileSize`
- [x] 3.4 Writeback: `TestE2E_WritebackFlushConsistency`, `TestE2E_WritebackReleaseRead`, `TestE2E_WritebackMultipleWritesThenRead`, `TestE2E_WritebackOverwriteAfterFsync` 等
- [x] 3.5 其他: `TestE2E_LargeFileMultiChunk`, `TestE2E_SequentialReadsDetectPattern`, `TestE2E_ChunkBoundaryRead`, `TestE2E_RandomAccessPattern`, `TestE2E_AccessAndTimes`, `TestE2E_Chmod`, `TestE2E_Utimens`, `TestE2E_Statfs`, `TestE2E_Mknod`, `TestE2E_FileInSubdirectory`, `TestE2E_WriteThenStat`

## 4. 待完成（与设计目标差距）

- [ ] 4.1 实现真实 FUSE 挂载 + 夸克 API E2E 测试（环境变量驱动，`QRYPT_TEST_COOKIE`/`QRYPT_TEST_MOUNT_POINT`）
- [ ] 4.2 实现持久化验证（写入 → 卸载 → 重新挂载 → 读取）
- [ ] 4.3 实现异步同步状态确定性反馈（`isDirty` 轮询验证上传完成）
- [ ] 4.4 在本地 macOS 环境下运行完整测试并归档变更
