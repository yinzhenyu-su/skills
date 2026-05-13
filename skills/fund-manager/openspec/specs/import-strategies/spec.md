## ADDED Requirements

### Requirement: Merge Strategy
系统必须支持在导入时将新金额合并（追加）到现有持仓中，这是默认行为。

#### Scenario: Importing an existing fund (default merge)
- **WHEN** 钱包已持有基金 A，且用户执行 `fund import "基金A" 5000`（未指定 override）
- **THEN** 系统插入一笔金额为 5000，类型为 `import` 的新交易记录

### Requirement: Override Strategy
系统必须支持使用 `--override` 标志清除特定基金在当前钱包的历史记录，并将导入金额作为唯一的新基准。

#### Scenario: Importing with override flag
- **WHEN** 钱包已持有基金 A 若干笔流水，用户执行 `fund import "基金A" 5000 --override`
- **THEN** 系统先删除该钱包下基金 A 的所有历史交易，然后插入一笔金额为 5000 的新交易

### Requirement: New Fund Behavior
对于钱包中不存在的新基金，无论哪种策略，行为必须一致。

#### Scenario: Importing a new fund
- **WHEN** 钱包中没有基金 B，用户执行 `fund import "基金B" 1000` (带或不带 --override)
- **THEN** 系统直接插入新记录，行为表现一致
