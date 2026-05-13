## ADDED Requirements

### Requirement: 内存 VFS 节点树
系统在内存中维护完整的 VFS 节点树，通过 `sync.Map` 实现并发安全的路径→节点对照。

#### Scenario: 节点存储
- **WHEN** 文件或目录被首次访问或创建
- **THEN** `storeNode(path, node)` 将该路径映射存入 `fs.nodes`（`sync.Map`）
- **AND** 若节点有非 `local_` 前缀的有效 fid，同时存入 `fs.fidNodes`（fid→节点映射）

#### Scenario: 路径变更
- **WHEN** 文件或目录被重命名
- **THEN** `replaceNodePath(oldPath, newPath, node)` 删除旧路径键、设置新路径键、更新父子关系
- **AND** 对于子节点，`recursiveRename` 递归更新所有后代节点的路径键

#### Scenario: 节点删除
- **WHEN** 文件或目录被删除
- **THEN** `deleteNodePath(path, node)` 从 `fs.nodes` 中移除路径键
- **AND** 从父节点的 `children` 映射中移除
- **AND** 若为目录，递归删除所有子节点的路径

### Requirement: Pending Node 记录 (Crash 恢复)
系统通过 `CacheManager.pendingNodes` 在内存中追踪需要异步上传的脏节点，用于启动时恢复。

#### Scenario: 脏节点持久化
- **WHEN** 文件创建或修改导致 `isDirty=true`
- **THEN** `SavePendingNode` 记录节点的 fid、路径、parentFid、文件名、本地 staging 路径、大小、Nonce、baseServerMtime、uploadID 等
- **AND** 此记录确保 crash 后启动时 `recoverDirtyFiles` 能重建 Node 并重新入队

#### Scenario: Pending 清理
- **WHEN** 上传成功完成
- **THEN** `RemovePendingNode(path)` 移除该路径的 pending 记录
- **WHEN** 目录被删除且有 pending 子节点
- **THEN** `RemovePendingNodesByPrefix(prefix)` 批量移除前缀匹配的记录

### Requirement: 启动恢复
系统启动时必须扫描 `pendingNodes`，对依然有效的脏节点重建 Node 并重新入队上传。

#### Scenario: 启动时节点恢复
- **WHEN** `NewFS` 调用 `recoverDirtyFiles()`
- **THEN** 遍历 `CacheManager.GetPendingNodes()`
- **AND** 对每个记录，尝试 `lookupExtended(path)` 查找已有节点
- **AND** 若节点不存在，但有 staging 文件，则重建 Node 并设置 `syncQueued=true` 后入队 `uploadChan`
- **AND** 若节点和 staging 文件都不存在，移除 pending 记录

### Requirement: 路径重建 (currentPathForNode)
系统提供 `currentPathForNode` 方法，通过递归遍历父子关系重建节点的完整路径，确保内存 VFS 树的路径一致性。

#### Scenario: 孤儿节点路径重建
- **WHEN** 某节点的 `currentPath` 为空（如被删除后重新发现）
- **THEN** `currentPathForNode` 沿 parentFid 链向上递归，重建完整路径
