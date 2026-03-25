## Why

fund-manager 目前能够记录交易、追踪持仓、同步净值，但缺乏帮助用户判断"当前能否买入/卖出"和"是否划算"的关键数据维度。用户在做买卖决策时需要临时去基金平台查申购状态、赎回费率、持有天数等信息，工具尚未成为真正的"个人基金经理"。把这些数据维度整合进来，是让 fund-manager 从"记账工具"升级为"决策辅助工具"的第一步。

## What Changes

- **新增**：基金可交易性信息展示，包括申购/赎回状态、单笔限购金额、最低买入额、资金到账天数
- **新增**：赎回费率分档结构，按持有天数计算实际赎回费用
- **新增**：持有天数计算与展示，在 `status` 和 `history` 命令中显示
- **新增**：卖出成本感知预览，`preview sell` 自动选取正确费率分档，展示实际到手金额
- **新增**：钱包内基金仓位占比，在 `status` 命令中显示每只基金占总资产的百分比
- **修改**：`fund inspect` 补充可交易性数据展示
- **修改**：数据源策略调整为多源提取：
 	- 申购/赎回状态优先使用东方财富 `lsjz` 的 `SGZT/SHZT`
 	- 分段费率优先使用 Morningstar `fees` 接口的 `redemptionFee/frontLoadFee/deferLoadFee`
 	- 限购与起购优先使用 Morningstar `purchaseAndRedeem/minInvestment`

## Capabilities

### New Capabilities

- `fund-tradability`: 基金可交易性数据，包括申购状态（开放/暂停/限额）、赎回状态（开放/暂停）、单笔限购金额、最低买入额、资金到账天数（T+N）
- `redemption-fee-tiers`: 基于持有天数的赎回费率分档，存储阶梯费率表（如持有 <7 天 1.5%、7-365 天 0.5%、>365 天 0%），`sell` 和 `preview sell` 命令据此自动计算实际费用
- `holding-period-display`: 在 `status` 和 `history` 命令中展示每笔买入的持有天数及当前适用赎回费率档位
- `portfolio-allocation`: 在 `status` 命令中计算并展示每只基金在当前钱包总资产中的仓位占比（百分比）

### Modified Capabilities

- `preview-subcommand`: `preview sell` 需在展示预计到手金额时，结合 `redemption-fee-tiers` 数据选取正确档位

## Impact

- `src/db.rs` — 新增 `fund_tradability` 表和 `redemption_fee_tiers` 表；`fund` 表新增字段
- `src/provider/eastmoney_lsjz.rs` — 扩展解析 `SGZT/SHZT` 并用于可交易状态
- `src/provider/morningstar.rs` — 扩展解析 `redemptionFee`、`deferLoadFee`、`purchaseAndRedeem` 等字段
- `src/main.rs` — `status` 命令新增仓位占比列；`sell` 和 `preview sell` 命令集成分档赎回费；`inspect` 命令展示可交易性数据
- `src/finance.rs` — 新增按持有天数查找赎回费率档位的计算函数
- `src/sync.rs` — `fund sync` 时同步更新可交易性数据
