## ADDED Requirements

### Requirement: 基金可交易性数据存储

系统 SHALL 将每只基金的申购状态、赎回状态、单笔限购金额、最低买入额、资金到账天数存储在 `fund_tradability` 表中。每次 `fund sync` 时更新。

字段来源约束：

- `subscription_status` / `redemption_status` 优先来自 Eastmoney `lsjz` 的 `SGZT/SHZT`
- `min_subscription_amount` / `limit_per_transaction` 优先来自 Morningstar `fees` 的 `minInvestment` 与 `purchaseAndRedeem`
- `settlement_days` 在无稳定来源时允许为 `NULL`

#### Scenario: 存储申购和赎回状态

- **WHEN** `fund sync` 为基金 `000300` 获取到申购状态为"开放申购"、赎回状态为"开放赎回"
- **THEN** 系统 SHALL 将这些状态写入 `fund_tradability` 表，关联 `fund_code = '000300'`

#### Scenario: 存储限购信息

- **WHEN** `fund sync` 获取到基金单笔限购上限为 10000 元、最低买入额为 100 元
- **THEN** 系统 SHALL 将 `limit_per_transaction = 10000`、`min_subscription_amount = 100` 写入 `fund_tradability` 表

#### Scenario: 存储资金到账天数

- **WHEN** `fund sync` 获取到赎回资金 T+2 工作日到账
- **THEN** 系统 SHALL 将 `settlement_days = 2` 写入 `fund_tradability` 表

#### Scenario: 到账天数无稳定来源时降级

- **WHEN** `fund sync` 无法获取到账天数（T+N）
- **THEN** 系统 SHALL 将 `settlement_days` 写为 `NULL` 并继续同步，不阻断流程

#### Scenario: API 数据不可用时优雅降级

- **WHEN** `fund sync` 时东方财富接口未返回可交易性字段
- **THEN** 系统 SHALL 保留上次已有数据（若有），或将可交易性字段标记为 NULL，不阻断同步流程

### Requirement: 可交易性信息在 inspect 命令中展示

`fund inspect` 命令 SHALL 在分析报告中新增"交易信息"区块，展示申购状态、赎回状态、最低买入额、单笔限购额和资金到账天数。

#### Scenario: 展示开放申购状态

- **WHEN** 用户运行 `fund-manager fund inspect 000300` 且该基金数据已同步
- **THEN** 输出表格中 SHALL 包含"申购状态: 开放申购"一行

#### Scenario: 展示暂停申购状态

- **WHEN** 某基金申购状态为"暂停申购"
- **THEN** `fund inspect` 输出 SHALL 显示"申购状态: ⚠️ 暂停申购"，并以醒目颜色（黄色或红色）标注

#### Scenario: 可交易性数据未同步时的提示

- **WHEN** 用户运行 `fund-manager fund inspect 000300` 但 `fund_tradability` 表中无该基金记录
- **THEN** 输出 SHALL 显示"交易信息: 数据不可用（运行 fund sync 更新）"

#### Scenario: 到账天数缺失时展示未知

- **WHEN** 用户运行 `fund-manager fund inspect 000300` 且该基金 `settlement_days` 为空
- **THEN** 输出 SHALL 在交易信息区显示"到账周期: 未知（以基金公司公告为准）"
