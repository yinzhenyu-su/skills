## MODIFIED Requirements

### Requirement: Sell Transaction Support
系统必须支持记录“卖出”类型的交易。

#### Scenario: Selling by shares
- **WHEN** 用户持有 1000 份基金，执行 `fund sell 000300 --shares 500`
- **THEN** 持仓份额减少 500，流水表增加一条 `sell` 类型的记录。

#### Scenario: Selling more than available
- **WHEN** 用户持有 1000 份，执行 `fund sell 000300 --shares 1200`
- **THEN** 系统必须报错，禁止该操作。

### Requirement: Sell with Auto-Calculation
系统应支持根据卖出金额自动计算所需卖出的份额。

#### Scenario: Selling by money
- **WHEN** 用户执行 `fund sell 000300 --money 1000 --auto`
- **THEN** 系统根据当前最新净值反算份额，并扣减持仓。
