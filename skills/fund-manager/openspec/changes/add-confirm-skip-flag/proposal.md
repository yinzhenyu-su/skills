## Why

当前工具在执行敏感操作（如删除基金、卖出份额等）时，会强制进行交互式二次确认。这虽然保证了安全性，但在脚本化运行、自动化测试或大批量快速操作场景下，交互式确认会阻断流程并降低效率。引入全局的 `-y` 或 `--yes` 参数，允许用户显式授权跳过确认，是提升 CLI 工具自动化能力的必要改进。

## What Changes

- **新增全局参数**：在顶层 CLI 定义中添加 `-y` / `--yes` 参数，并标记为 `global = true`。
- **重构确认逻辑**：将现有的交互式确认代码抽象或修改，使其在 `cli.yes` 为 `true` 时直接返回 `true`。
- **操作反馈优化**：当跳过确认时，在控制台输出提示信息，告知用户正在执行已授权的敏感操作。

## Capabilities

### New Capabilities
- `confirm-bypass`: 提供一种非交互式的方式来执行原本需要二次确认的敏感指令。

### Modified Capabilities
- `wallet-management`: 准备支持未来的钱包删除确认跳过。
- `transaction-tracking`: 使 `sell` 命令（及未来的变动操作）支持确认跳过。
- `fund-deletion`: 使 `fund delete` 命令支持确认跳过。

## Impact

- **CLI 定义**：修改 `src/cli.rs` 以包含顶层全局参数。
- **应用逻辑**：修改 `src/main.rs` 中处理 `fund delete` 和其他未来确认逻辑的地方。
- **用户体验**：提升了自动化集成能力。
