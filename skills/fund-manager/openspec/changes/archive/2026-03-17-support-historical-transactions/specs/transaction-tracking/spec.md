## MODIFIED Requirements

### Requirement: Context Override by Name
`buy` 和 `sell` 命令必须支持显式指定钱包名称以覆盖活跃设置，并允许指定交易日期。

#### Scenario: Transaction with explicit wallet and date
- **WHEN** 当前活跃钱包是 "A"，但用户执行 `fund buy 000300 --wallet "B" --date "2024-01-01"`
- **THEN** 交易必须被记录在钱包 "B" 下，日期为 "2024-01-01"，且不改变全局活跃钱包。
