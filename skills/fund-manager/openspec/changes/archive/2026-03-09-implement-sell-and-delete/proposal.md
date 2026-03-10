## Why

当前用户只能通过 `buy` 命令增加基金份额，无法记录卖出（赎回）操作，导致持仓数据无法闭环。同时，用户也需要一种方式来彻底移除不再关注的基金及其所有历史数据，以保持数据库的整洁。

## What Changes

- **实现 `sell` 命令**：支持记录基金赎回操作，自动更新持仓份额，并计算赎回金额。
- **实现基金删除功能**：支持通过基金代码删除特定基金的所有持仓记录、历史净值和交易流水。
- **增强数据一致性**：确保卖出操作后的份额不会变成负数，并在删除时处理好外键关联数据。

## Capabilities

### New Capabilities
- `fund-deletion`: 提供彻底删除基金相关数据（包括流水和净值历史）的能力。

### Modified Capabilities
- `transaction-tracking`: 扩展交易追踪能力，增加对“卖出”类型的支持，并实现份额扣减逻辑。

## Impact

- **数据库**：`transaction_log` 表将增加更多 `sell` 类型的记录；删除操作将触及 `fund`, `transaction_log` 和 `nav_history` 表。
- **CLI**：新增 `fund delete <code>` 子命令，扩展 `buy` 命令以外的交易操作。
