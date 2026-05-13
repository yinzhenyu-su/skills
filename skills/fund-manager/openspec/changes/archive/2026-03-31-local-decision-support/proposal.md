## Why

fund-manager 目前仅能作为被动记账工具，用户在执行交易决策（如是否现在卖出、当前是否保本、何时止盈）时仍需依赖外部计算或直觉。通过引入本地决策支持，可以利用现有的阶梯费率和交易数据，为用户提供低延迟、高确定性的操作建议。

## What Changes

- **新增**：在 `status` 命令中展示“保本净值”列，反映考虑预估手续费后的平衡点。
- **新增**：在 `status` 和 `preview sell` 命令中增加“阶梯费率提醒”，检测是否临近降档日（如再持有 1 天手续费减半）。
- **新增**：基金配置中支持设置个人收益目标（止盈/止损百分比）。
- **新增**：在 `status` 中通过直观符号（💡/🎯/⚠️）标记决策建议。

## Capabilities

### New Capabilities

- `breakeven-calculation`: 计算并展示保本净值，公式考虑买入成本、已持有份额及当前对应的赎回费率档位。
- `holding-optimization`: 基于赎回费率阶梯，计算当前持有天数与下一降档日期的距离，并给出节省费用的量化建议。
- `profit-loss-alerts`: 支持为每只基金配置 `target_profit_rate` 和 `stop_loss_rate`，并在 `status` 输出中实现报警提醒。

### Modified Capabilities

- `fund-valuation-display`: `status` 命令的展示逻辑需要扩展，以包含保本净值、决策标记和汇总提醒文案。
- `preview-subcommand`: `preview sell` 命令在输出中集成持有期优化建议，提醒用户是否应推迟卖出以节省费用。

## Impact

- `src/db.rs` — `fund` 表新增 `target_profit_rate` 和 `stop_loss_rate` 字段；新增查询下一档费率的函数。
- `src/finance.rs` — 新增保本净值计算逻辑及费率跳档检测函数。
- `src/main.rs` — 修改 `status` 和 `preview sell` 的表格展示逻辑；增加对 `fund config` 命令参数的解析。
- `src/cli.rs` — 为 `fund config` 子命令增加可选的止盈止损参数。
