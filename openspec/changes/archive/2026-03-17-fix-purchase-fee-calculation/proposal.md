## Why

当前 `fund-manager` 在处理 `buy` 命令时，错误地使用基金的**管理费率**（年度费用）代替**申购费率**（交易费用）进行计算，且在解析费率百分比时存在逻辑错误（如将 "0.15%" 解析为 0.15 即 15%），导致申购手续费计算结果严重偏高，影响了持仓成本和份额的准确性。

## What Changes

- **修复费率解析逻辑**：在解析以 `%` 结尾的费率字符串时，确保正确转换为小数（例如 "0.15%" 应转换为 0.0015）。
- **优化费率字段使用**：在 `buy` 命令中，优先使用 `sales_fee`（申购费率）而非 `management_fee`（管理费率）。
- **完善财务计算函数**：确保 `src/finance.rs` 中的计算逻辑符合公认的基金申购计算公式。

## Capabilities

### Modified Capabilities
- `transaction-tracking`: 修正申购交易中手续费和份额的自动计算规则，确保费率解析和字段选择的正确性。

## Impact

- `skills/fund-manager/src/finance.rs`: 修复费率百分比解析逻辑。
- `skills/fund-manager/src/main.rs`: 调整 `buy` 命令逻辑，优先获取 `sales_fee` 并传递给计算函数。
- `skills/fund-manager/src/db.rs` & `skills/fund-manager/src/provider/`: 确认费率相关字段的定义和抓取是否支持申购费率。
