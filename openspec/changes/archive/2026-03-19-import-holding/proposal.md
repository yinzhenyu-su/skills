## Why

用户希望从其他平台（支付宝、天天基金、微信理财通等）导入基金持仓数据。这些平台通常支持导出"基金名称、持有金额、持有收益"三个字段，但无法直接导出"持有份额"或"买入时的净值"。需要系统能够根据导入的持仓数据反推成本和份额，以追踪基金的实时涨跌。

此外，用户在买入或卖出前希望能够预览交易结果，了解手续费、份额变化和持仓成本均价的变化。

## What Changes

1. **扩展 `transaction_log` 支持 `import` 类型**：导入的持仓以 `type='import'` 存入交易日志，与后续的买入卖出统一管理
2. **新增 `fund import-holding` 命令**：支持 CSV 批量导入持仓数据（基金名称、持有金额、持有收益），导入时自动计算份额和成本
3. **新增 `fund preview buy` 命令**：预览买入结果，展示手续费、获得份额、买入后持仓变化（总份额、总成本、成本均价）
4. **新增 `fund preview sell` 命令**：预览卖出结果，展示赎回金额、手续费、卖出后持仓变化
5. **复用现有 `fund status` 命令**：展示导入持仓与后续买卖汇总后的整体盈亏

**关键设计决策**：
- 导入记录直接写入 `transaction_log`（type='import'），而非新建表
- 后续买入卖出正常记录，与导入数据合并计算，统一展示盈亏
- 用户只需关心"整体持仓盈亏"，无需区分数据来源

## Capabilities

### New Capabilities

- `holdings-import`: 新建持仓导入能力，支持通过 CSV 导入持仓数据，根据持有金额和持有收益反推成本和份额，写入 transaction_log
- `transaction-preview`: 新建交易预览能力，支持买入和卖出前预览，展示手续费、份额变化、成本均价变化

### Modified Capabilities

- (无)

## Impact

- 复用现有 `transaction_log` 表（无需新建表）
- 新增 `fund import-holding` CLI 命令
- 新增 `fund preview buy` CLI 命令
- 新增 `fund preview sell` CLI 命令
- 需要获取净值（调用现有 provider）
