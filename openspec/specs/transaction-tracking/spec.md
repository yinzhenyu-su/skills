## MODIFIED Requirements

### Requirement: Context Override by Name
`buy` 和 `sell` 命令必须支持显式指定钱包名称以覆盖活跃设置，并允许指定交易日期。

#### Scenario: Transaction with explicit wallet and date
- **WHEN** 当前活跃钱包是 "A"，但用户执行 `fund buy 000300 --wallet "B" --date "2024-01-01"`
- **THEN** 交易必须被记录在钱包 "B" 下，日期为 "2024-01-01"，且不改变全局活跃钱包。

### Requirement: Import Transaction Support
系统必须支持记录 `import` 类型的交易，其在统计计算（如成本、估值、份额）中的方向应与 `buy` 保持一致，代表资产的增加。

#### Scenario: Aggregation of holdings
- **WHEN** 计算钱包的持仓时，遇到 `type = 'import'` 的记录
- **THEN** 该记录的份额和金额必须被视为正数计入总持仓和总成本
