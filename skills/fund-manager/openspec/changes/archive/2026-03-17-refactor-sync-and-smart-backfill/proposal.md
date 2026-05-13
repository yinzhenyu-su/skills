## Why

当前 `fund fund sync` 命令存在逻辑模糊（无参运行行为不明确）和硬编码限制（`--all` 模式强制 30 天回溯），导致用户难以高效地处理历史 `pending` 交易。通过重构，我们可以建立更清晰的业务逻辑，并实现自动化的“智能回溯”，确保所有待结算交易都能通过一次同步得到平账。

## What Changes

- **规范同步命令行为 (BREAKING)**：移除 `fund fund sync` 的无参隐式全量同步行为。用户必须显式指定基金代码或使用 `--all` 标志。
- **引入智能回溯逻辑**：`sync --all` 和 `sync <fund>` 不再仅依赖于硬编码的 30 天或用户输入的 `--start`。系统将自动扫描数据库中最早的 `pending` 交易日期，并以此作为同步起点。
- **优化全量同步流程**：确保 `sync --all` 在遍历基金时，能高效地处理元数据更新和净值补全。
- **增强用户反馈**：在同步过程中提供清晰的任务进度指示（例如 `[1/5] Syncing 000300...`）。

## Capabilities

### New Capabilities
- `smart-settlement-sync`: 自动根据数据库中 `pending` 状态的交易记录，智能推导同步的时间范围起点，实现“自动对账”。

## Impact

- `skills/fund-manager/src/main.rs`: 重构 `Sync` 命令的分支处理逻辑，强化参数校验。
- `skills/fund-manager/src/sync.rs`: 实现智能时间窗口推导算法，并优化进度打印输出。
- `skills/fund-manager/src/db.rs`: 新增查询接口，用于获取指定基金或全局范围内最早的 `pending` 交易日期。
