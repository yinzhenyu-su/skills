## ADDED Requirements

### Requirement: Transaction Recording with Auto-Shares
系统应支持根据买入金额和当前最新净值/费率自动计算份额。

#### Scenario: Buying with auto-calculation
- **WHEN** 用户执行 `fund buy 000300 --money 1000 --auto`，且系统已知最新净值为 1.00，费率为 0.1%
- **THEN** 系统计算扣除 1 元手续费，生成持仓份额 999 份，并向流水表写入一条买入记录。

### Requirement: Fund Search by Code or Name
系统必须支持在命令中混用基金名称或代码进行检索。

#### Scenario: Searching by name
- **WHEN** 用户执行 `fund status "沪深300"`
- **THEN** 系统应通过名称查找匹配的基金代码，并显示其持仓状态。
