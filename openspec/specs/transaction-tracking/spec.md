## MODIFIED Requirements

### Requirement: Context Override by Name
`buy` 和 `sell` 命令 must 支持显式指定钱包名称以覆盖活跃设置，并允许指定交易日期。

#### Scenario: Transaction with explicit wallet and date
- **WHEN** 当前活跃钱包是 "A"，但用户执行 `fund buy 000300 --wallet "B" --date "2024-01-01"`
- **THEN** 交易必须被记录在钱包 "B" 下，日期为 "2024-01-01"，且不改变全局活跃钱包。

### Requirement: Import Transaction Support
系统 must 支持记录 `import` 类型的交易，其在统计计算（如成本、估值、份额）中的方向应与 `buy` 保持一致，代表资产的增加。

#### Scenario: Aggregation of holdings
- **WHEN** 计算钱包的持仓时，遇到 `type = 'import'` 的记录
- **THEN** 该记录的份额和金额 must 被视为正数计入总持仓和总成本

## ADDED Requirements

### Requirement: Correct Fee Rate Parsing
系统在处理以 `%` 结尾的费率字符串时，must 将其正确转换为小数形式（即除以 100）。

#### Scenario: Parsing percentage fee rate
- **WHEN** 调用费率解析函数处理字符串 "0.15%"
- **THEN** 得到的内部十进制数值 must 等于 0.0015。

### Requirement: Purchase Fee Field Priority
在执行 `buy` 命令进行自动计算时，系统 must 优先寻找并使用基金的申购费率（`sales_fee`）。如果 `sales_fee` 不可用，则回退到管理费率（`management_fee`）或系统默认费率（0.15%），并应在输出中明确告知用户使用了哪种费率。

#### Scenario: Automatic fee calculation with prioritized field
- **WHEN** 执行 `buy` 命令且基金定义中同时存在 `sales_fee` ("0.10%") 和 `management_fee` ("0.15%")
- **THEN** 计算过程 must 采用 0.10% 作为费率。

### Requirement: Standard Fund Purchase Formula Accuracy
手续费（Fee）和份额（Shares）的计算 must 严格遵循公认的基金申购公式：$NetAmount = TotalMoney / (1 + FeeRate)$，$Fee = TotalMoney - NetAmount$。计算过程中 must 注意舍入精度，手续费通常保留两位小数，份额保留两位小数。

#### Scenario: Calculating purchase with standard formula
- **WHEN** 用户以 100 元买入净值为 2.3026 的基金，申购费率为 0.15%
- **THEN** 手续费应计算为 0.15 元（100 - 100/1.0015 ≈ 0.1497，舍入为 0.15），最终份额应为 37.77 份（(100-0.15)/2.3026 ≈ 37.7659，舍入为 37.77）。
