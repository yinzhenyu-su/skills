## Why

qrypt 已具备删除与修改文件的基础实现，但当前规范、状态一致性策略和验证闭环仍不完整，导致功能可用性与可维护性存在风险。现在需要把“能用”收敛为“稳定可预期”，避免删除/修改在异常场景下出现残留任务、状态漂移或行为不一致。

## What Changes

- 明确并固化删除语义：删除远端对象时，必须同步清理本地 dirty/pending 状态与相关缓存，避免重试任务复活已删除文件。
- 明确并固化修改语义：以写回模型为准，定义 Write/Flush 及失败重试下的状态转换与成功判定。
- 增加上传闭环要求：修改上传路径必须包含加密对象 hash 上报步骤，并定义 finish=true 的终止条件。
- 增加可靠性与恢复要求：重启恢复时仅恢复有效 pending 任务，跳过已删除或无效状态对象。
- 增加最小验证矩阵：覆盖删除、覆盖写、跨目录重命名后的修改、失败重试与重启恢复。

## Capabilities

### New Capabilities

- `qrypt-delete-consistency`: 定义删除操作与本地 dirty/pending/缓存状态一致性的行为契约。
- `qrypt-modify-writeback-stability`: 定义修改文件写回上传的状态机、上传闭环和异常恢复行为。

### Modified Capabilities

- 无。

## Impact

- 受影响模块：skills/qrypt/internal/vfs、skills/qrypt/internal/driver、skills/qrypt/internal/cache。
- 受影响流程：Unlink/Rmdir、Write/Flush/syncFile、重启恢复 pending 任务。
- 对外影响：删除与修改行为从“实现可用”升级为“规范化可验证”，降低线上不一致风险。
