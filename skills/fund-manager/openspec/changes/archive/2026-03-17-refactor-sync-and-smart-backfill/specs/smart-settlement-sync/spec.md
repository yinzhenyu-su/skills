## ADDED Requirements

### Requirement: Command Argument Strictness
`sync` 命令必须要求用户显式提供基金标识符或使用 `--all` 标志。严禁在没有任何参数且无标志的情况下隐式执行任何同步操作，以防止逻辑混淆。

#### Scenario: Running sync without mandatory arguments
- **WHEN** 用户执行 `fund fund sync` 且既不提供基金代码也不提供 `--all` 标志
- **THEN** 系统必须报错，并给出明确提示：“请指定基金代码或使用 --all 进行全量同步”。

### Requirement: Smart Sync Start Date推导
同步命令的起始日期（Start Date）必须具备智能推导能力。推导优先级顺序如下：
1. 用户显式通过 `--start` 指定的日期。
2. 数据库中当前相关基金（或全局，若使用 `--all`）最早的 `pending` 状态交易日期。
3. 默认值：对于 `sync --all`，默认使用 7 天；对于单只基金同步且无 `pending` 记录，默认仅同步最新数据。

#### Scenario: Smart sync triggered by pending records
- **WHEN** 数据库中存在一笔日期为 2024-01-10 的 `pending` 买入记录，用户执行 `fund fund sync --all` 且未提供 `--start`
- **THEN** 系统的同步起始点必须自动设定为 2024-01-10，以确保该 `pending` 交易能够得到结算。

#### Scenario: Explicit start date overrides smart logic
- **WHEN** 数据库中存在 2024-01-10 的 `pending` 记录，但用户执行 `fund fund sync --all --start 2024-02-01`
- **THEN** 系统必须遵循显式指令，从 2024-02-01 开始同步。

### Requirement: Batch Sync Progress Feedback
在全量同步或多基金同步场景下，系统必须向用户提供明确的进度反馈。

#### Scenario: Feedback during all funds sync
- **WHEN** 用户执行 `fund fund sync --all` 且共有 5 只基金
- **THEN** 系统在处理每只基金前，必须打印进度提示（如 `[3/5] Syncing 000300 (沪深300)...`）。
