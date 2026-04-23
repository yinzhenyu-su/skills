# 内存节点转换与上传逻辑优化报告

### 问题深度分析：为什么会出现 `Local Conflict`？

在 `dist/favicon.ico` 的案例中，产生冲突的核心原因在于**内存状态更新的非原子性**与**最终一致性系统的时序竞争**。

#### 1.1 全局索引 `fidNodes` 更新缺失

这是目前代码中的一个关键疏漏：

- **现状**：`syncFile` 在上传成功后，更新了 `node.fid`，但**没有更新 `fs.fidNodes` 这个全局反查索引**。
- **后果**：`MergeRemoteChanges` 通过 `fidNodes.Load(remoteFID)` 寻找匹配的本地节点。由于索引里存的还是旧的 `local_xxx`，它会认为这个 `remoteFID` 是一个全新的文件。随后触发“远端有，本地无”的逻辑，试图添加新节点，发现路径被占用了，于是强制将本地节点改名为 `[Local Conflict]`。

#### 1.2 状态转换的“真空期”

- **现状**：节点从 `local` 到 `remote` 的身份转变被分割在两个环节。`syncFile` 负责物理上传，`MergeRemoteChanges` 负责逻辑确认。
- **风险**：在文件传完到下一次 `ls` 刷新之间，节点处于“已上传但身份未转正”的中间态。如果此时刷新列表，硬编码的 30s 时间保护是唯一的防线。

#### 1.3 `lastUploadTime` 的不可靠性

- **现状**：这是一个纯内存字段。一旦程序在上传完成后、索引确认前重启，该时间将丢失（归零）。
- **后果**：重启后的第一次同步，由于 `lastUpload.IsZero()` 为真，30s 保护逻辑会被绕过，导致系统直接判定为冲突。

---

### 改进方案：基于“归属验证”的事件驱动逻辑

#### 方案 A：实现内存索引同步（立即见效）

在 `syncFile` 更新 FID 的原子操作中，必须同步维护全局索引：

1. `fs.fidNodes.Delete(oldLocalFid)`
2. `fs.fidNodes.Store(newRemoteFid, n)`
这能保证 `MergeRemoteChanges` 刷新时能立刻认出“自己人”。

#### 方案 B：引入 `ExpectedFID` 追踪（彻底消除 30s 依赖）

将“基于时间的猜测”改为“基于数据的确证”：

1. **记录意图**：在 `syncFile` 上传成功后，在 `node` 结构体中记录本次上传得到的 `expectedFid`。
2. **证据匹配**：在 `MergeRemoteChanges` 中，如果发现重名但 FID 不匹配，首先检查该远端 FID 是否等于该节点的 `expectedFid`。如果是，说明它是异步索引延迟后的回显，直接执行 FID 替换和 `source` 转正，严禁触发冲突改名。

#### 方案 C：状态原子化转换

将 `n.source` 的转换时机提前：

- `syncFile` 成功后立即设置：`n.source = "remote"`, `n.isDirty = false`, `n.lastUploadTime = now`。
- 配合方案 B，即使 `MergeRemoteChanges` 在一秒后运行，也能确保逻辑链条的完整。
