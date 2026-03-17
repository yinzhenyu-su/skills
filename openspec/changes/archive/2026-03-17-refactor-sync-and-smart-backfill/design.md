## Context

当前的 `Sync` 逻辑中，起始日期的计算不透明且存在多处重叠：
1. `main.rs` 在全量同步模式下硬编码了 `30` 天。
2. `sync.rs` 内部又有复杂的 `calculate_sync_range` 函数。
3. 全量同步和单只基金同步在元数据更新上表现不一致。

## Goals / Non-Goals

**Goals:**
- 统一全量同步和局部同步的行为模式。
- 实现基于 `pending` 状态的智能日期对齐。
- 提供更具交互感的任务进度反馈。

**Non-Goals:**
- 不修改 `sync_funds` 本身的多 provider 聚合逻辑。
- 不增加外部配置文件。

## Decisions

- **职责迁移**：将起始日期（Start Date）的推导核心逻辑收拢。在 `db.rs` 中提供原始数据查询能力。
- **智能推导算法**：
    - 全量同步：`start = user_provided_start ?? earliest_pending_of_all_funds ?? today - 7 days`。
    - 局部同步：`start = user_provided_start ?? earliest_pending_of_specific_fund ?? today`。
- **命令接口调整**：强制要求参数显性化。如果输入 `fund fund sync` 没有任何其他参数，则抛出错误。
- **进度指示器**：在同步循环中引入计数器（例如 `i+1 / total`），打印每一步的详细状态。

## Risks / Trade-offs

- **[Risk] 数据库中 pending 记录日期极早导致同步风暴** → **[Mitigation]** 为智能回溯设置合理的上限（如 1 年），或者在检测到时间跨度超过 30 天时向用户输出一条 Warning 提示。
- **[Trade-off] 元数据更新频率** → `--all` 模式保持原有的元数据更新逻辑，而单只基金同步依然侧重于净值，以保持操作的快速响应。
