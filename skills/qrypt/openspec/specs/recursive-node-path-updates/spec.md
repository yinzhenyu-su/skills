## ADDED Requirements

### Requirement: Readdir 合并远程变更时递归路径更新
系统在 `Readdir` 中调用 `MergeRemoteChanges` 时，如果检测到已有本地节点的远程 fid 变化（如远程重命名后），通过 `replaceNodePath` 和 `recursiveRename` 递归更新路径。

### Requirement: Rename 操作递归路径更新
当目录被重命名时，系统必须递归更新其所有子孙节点在内存 VFS 树中的路径键。

#### Scenario: 目录重命名
- **WHEN** 用户将目录 `/A` 重命名为 `/X`
- **AND** `/A` 包含文件 `/A/b.txt`
- **THEN** `QryptFS.Rename` 调用 `replaceNodePath("/A", "/X", nodeA)`
- **AND** 调用 `recursiveRename("/A", "/X", nodeA)`
- **AND** `recursiveRename` 递归遍历所有子节点，对每个子节点调用 `replaceNodePath` 更新路径
- **AND** 最终 `/A/b.txt` 的路径更新为 `/X/b.txt`

#### Scenario: 重命名 + Pending 节点
- **WHEN** 目录 `/A` 包含一个 dirty 子文件且已入队等待上传
- **AND** 目录被重命名为 `/X`
- **THEN** `persistPendingPath(oldPath, newPath, node)` 同步更新 `pendingNodes` 中的路径索引
- **AND** 后续 `syncFile` 中的路径检查（`currentPath != path`）不会误判

### Requirement: 删除时路径清理
当节点被删除时，系统必须清理其 `currentPath` 并从 `pendingNodes` 中移除关联记录。

#### Scenario: 删除时路径清理
- **WHEN** 文件被 Unlink 或在非空目录 Rmdir 时
- **THEN** `deleteNodePath(path, node)` 标记 `node.currentPath = ""`
- **AND** 在 `cleanupLocalUploadState` 中通过 `RemovePendingNode` 移除 pending 记录

### Requirement: 路径重建兜底
系统通过 `currentPathForNode` 方法，在启动恢复等场景下沿 parentFid 链递归重建节点的完整路径，确保内存 VFS 树不产生路径孤岛。

#### Scenario: 节点路径重建
- **WHEN** `storeNode` 遇到 `currentPath` 为空或有冲突的节点
- **THEN** `currentPathForNode` 沿 parentFid 向上递归，标记路径前缀
- **AND** 组装后的路径存入 `nodes` 映射，确保路径索引一致
