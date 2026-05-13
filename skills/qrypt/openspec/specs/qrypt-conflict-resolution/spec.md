## ADDED Requirements

### Requirement: Conflict Detection in syncFile
`syncFile` 在每次执行上传前，通过两个检查检测冲突：

**检查 1 — 同名不同 fid**: 父目录中存在与本地同名的远程文件，但其 fid 与节点的 `currentFid` 不匹配，说明其他客户端已上传了同名文件。
**检查 2 — 本地 fid 消失**: 节点的 `currentFid` 在服务端父目录中不存在了，说明远程文件已被删除或替换。

#### Scenario: 冲突检测触发 side-by-side resolve
- **WHEN** `syncFile` 检测到上述任一冲突条件
- **THEN** 调用 `resolveConflict(path, node, remoteFile)`
- **AND** `resolveConflict` 将本地 dirty 节点重命名为 `path [Local Conflict YYYYMMDD_HHMMSS].ext`
- **AND** 使用 `replaceNodePath` 和 `persistPendingPath` 更新 VFS 树和 pending 记录
- **AND** 为冲突节点分配新的 local_ fid 和新的 File Nonce
- **AND** 原始的 `path` 位置被替换为远程文件的只读 Node（`source="remote"`），保留远程 fid 和加密尺寸

### Requirement: 10 秒上传冷却作为冲突屏障
`syncFile` 在冲突检查前设置 10 秒冷却期，确保前一个上传操作的 `deleteExistingFileByName` 和 fid 更新已完全体现在服务端。

#### Scenario: Rapid edits with cooldown
- **WHEN** 同一文件的两次 Release 间隔不足 10 秒
- **THEN** 第二次 `syncFile` 在 10 秒冷却期内返回 `nil`，`defer` 重新入队
- **AND** 10 秒后重新读取 `currentFid` 和执行冲突检测，避免使用过时的 snapshot fid 产生假阳性冲突

### Requirement: Conflict tracking per node
每个 `Node` 结构体 SHALL 维护以下字段用于冲突跟踪：
- `baseServerMtime int64` — 上次成功同步时服务端的 `updated_at` (毫秒时间戳)
- `baseServerSize int64` — 上次成功同步时服务端的文件大小
- `lastUploadTime time.Time` — 上次发起上传的时刻，用于 10 秒上传冷却检测

### Requirement: Delete-before-upload to prevent conflicts
上传前，系统必须通过 `deleteExistingFileByName` 按明文文件名查找并删除远程已存在的同名文件，防止夸克网盘自动产生 `(1)` 冲突副本。
