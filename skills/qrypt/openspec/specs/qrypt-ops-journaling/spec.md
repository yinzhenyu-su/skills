## ADDED Requirements

### Requirement: Operations Logging (JSONL journal)

系统 SHALL 在 `CacheManager` 中使用 JSONL（每行一个 JSON 对象）文件记录元数据操作日志。日志文件存放在缓存目录下的 `ops_log.jsonl`。

#### Scenario: Record metadata operation
- **WHEN** `processBatchMetadataTasks` 处理 DELETE 操作（Rename / Delete / Mkdir）
- **THEN** 系统在远程 API 调用前，先调用 `AppendOpsLog` 写入一条 `OpsLogEntry`
- **AND** `OpsLogEntry` 包含：`op`（操作类型）、`path`（路径）、`fid`（目标 fids）、`ts`（时间戳）、`done`（是否完成）

```go
type OpsLogEntry struct {
    OpType    string `json:"op"`      // "DELETE", "RENAME", "MKDIR"
    Path      string `json:"path"`
    Fid       string `json:"fid,omitempty"`
    Timestamp int64  `json:"ts"`
    Done      bool   `json:"done"`
}
```

#### Scenario: Mark as done
- **WHEN** 远程 API 调用成功
- **THEN** `processBatchMetadataTasks` 调用 `MarkOpsLogDone(path)` 标记该记录为已完成
- **AND** 从 JSONL 文件中重写该记录（`done=true`）

### Requirement: Recovery of Pending Operations

系统启动时 SHALL 扫描 `ops_log` 中 `done=false` 的记录，并重放未完成的元数据操作。

#### Scenario: Recovery after crash
- **WHEN** `NewFS` 在启动时调用 `replayOpsLog()`
- **THEN** 扫描 ops_log，对每个 `done=false` 的记录：
  - **IF** 操作类型为 `DELETE` — 检查 `activeDeletions` 中目标 fid 是否仍存在，若是则重新执行远程删除
  - **AND** 操作执行完成后标记为 `done=true`
- **AND** 重放完成后，调用 `PurgeOpsLog(72h)` 清理 72 小时前的已完成记录

### Requirement: Ops log maintenance

系统 SHALL 在每次 `replayOpsLog` 完成后执行清理，`PurgeOpsLog` 删除超过 72 小时的已完成记录，防止日志文件无限增长。

### Additional: Path tracking for rename resilience

对于重命名操作，系统除了 ops_log 记录外，还通过 `persistPendingPath(oldPath, newPath, node)` 在 `pendingNodes` 中同步更新路径索引，确保即使 crash 后恢复，节点路径映射也能正确重建。
