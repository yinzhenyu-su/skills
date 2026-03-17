## ADDED Requirements

### Requirement: Import Transaction Support
系统必须支持记录 `import` 类型的交易，其在统计计算（如成本、估值、份额）中的方向应与 `buy` 保持一致，代表资产的增加。

#### Scenario: Aggregation of holdings
- **WHEN** 计算钱包的持仓时，遇到 `type = 'import'` 的记录
- **THEN** 该记录的份额和金额必须被视为正数计入总持仓和总成本
