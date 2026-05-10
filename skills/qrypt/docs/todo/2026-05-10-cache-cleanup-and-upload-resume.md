## 两个功能的分析和实施计划

### Feature 1: 7天缓存清理

**现状:**
- `chunks` 表: 151,783 行，最旧记录仅 1 天
- 数据库 41MB，99.9% 是 chunks 表
- `Maintenance()` 每 10 分钟运行一次，但 30 天硬删除阈值太高
- `EvictIfNeeded` 清理空间的 batch 文件，但不按时间维度清理
- `GetChunkPathsByFid` 可以拿到 fid 关联的 batch 文件路径

**实施:**
1. `db.go` 新增 `CleanupOldChunks(days int) (deletedChunks int, deletedFiles int64, err error)` 
   - SELECT chunks 找 `access_time < datetime('now', ?)` 
   - 收集去重的 batch 文件路径
   - 先删物理文件，再删 DB 记录
   - 返回清理统计
2. `manager.go` 在 `Maintenance()` 中调用，加在 `EvictIfNeeded` 之后
3. 清理后 `VACUUM` 回收空间

### Feature 2: 上传断点续传

**现状:**
- `nonce` 已入库 (`pending_nodes.file_nonce`) 但从不复用
- `upload_id` 由 UploadPre 返回但从不持久化
- 崩溃后 `recoverDirtyFiles` 重新入队，但生成新 nonce → 新加密数据 → 旧 OSS parts + placeholder 全部作废

**方案: 持久化 upload_id + 复用 nonce**

| 阶段 | 操作 |
|------|------|
| UploadPre 后 | 保存 `upload_id` 到 pending_nodes |
| UploadPart 循环中 | 每 25 个 part 更新一次 `last_part` |
| 成功完成 | `RemovePendingNode` 清掉所有记录（已有） |
| 崩溃重启 | `recoverDirtyFiles` 恢复 `upload_id` + 旧 nonce |
| 续传上传 | Sync 拿到旧 nonce → 不解密生成的相同加密数据 → 旧 `upload_id` 传给 UploadPre 续传同一 session → 从 `last_part+1` 开始传 |

**关键设计决策:**
- 同 nonce → 同加密数据 → OSS 同 `upload_id` + `part_number` = 幂等覆盖
- 续传时调用 `UploadPre(old_upload_id)` → Quark 识别为续传
- 前 `last_part` 个 part 跳过不上传（OSS 已有）
- `last_part` 每 25 个 part 或每 5 秒更新一次（避免频繁 SQLite 写入）
