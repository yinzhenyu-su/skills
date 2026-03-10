## MODIFIED Requirements

### Requirement: Context Override by Name
`buy` 和 `sell` 命令必须支持显式指定钱包名称以覆盖活跃设置。

#### Scenario: Transaction with explicit wallet
- **WHEN** 当前活跃钱包是 "A"，但用户执行 `fund buy 000300 --wallet "B"`
- **THEN** 交易必须被记录在钱包 "B" 下，且不改变全局活跃钱包为 "B"。

### Requirement: Sell Validation for Non-existent
如果卖出操作指定的基金在本地数据库中完全没有记录，必须报错。

#### Scenario: Selling non-existent fund
- **WHEN** 用户执行 `fund sell 999999` 且该代码本地不存在
- **THEN** 系统报错：“你从未追踪过此基金，无法执行卖出操作”。
