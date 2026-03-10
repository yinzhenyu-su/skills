## ADDED Requirements

### Requirement: Holding Valuation Calculation
系统应基于持仓总量和最新单位净值，动态计算持仓估值。

#### Scenario: Calculating valuation
- **WHEN** 用户拥有 1000 份基金，且其最新净值为 1.2
- **THEN** 持仓状态应显示估值为 1200 元。

### Requirement: Profit and Loss Calculation
系统必须根据买入流水记录的成本和当前持仓估值，计算出浮动盈亏（金额和比例）。

#### Scenario: Displaying P&L
- **WHEN** 用户持有总成本为 1000 元，当前估值为 1200 元
- **THEN** 系统显示浮盈 200 元，涨跌幅 20%。
