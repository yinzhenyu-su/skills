## ADDED Requirements

### Requirement: Correct Fee Rate Parsing
系统在处理以 `%` 结尾的费率字符串时，必须将其正确转换为小数形式（即除以 100）。

#### Scenario: Parsing percentage fee rate
- **WHEN** 调用费率解析函数处理字符串 "0.15%"
- **THEN** 得到的内部十进制数值必须等于 0.0015。

### Requirement: Purchase Fee Field Priority
在执行 `buy` 命令进行自动计算时，系统必须优先寻找并使用基金的申购费率（`sales_fee`）。如果 `sales_fee` 不可用，则回退到管理费率（`management_fee`）或系统默认费率（0.15%），并应在输出中明确告知用户使用了哪种费率。

#### Scenario: Automatic fee calculation with prioritized field
- **WHEN** 执行 `buy` 命令且基金定义中同时存在 `sales_fee` ("0.10%") 和 `management_fee` ("0.15%")
- **THEN** 计算过程必须采用 0.10% 作为费率。

### Requirement: Standard Fund Purchase Formula Accuracy
手续费（Fee）和份额（Shares）的计算必须严格遵循公认的基金申购公式：$NetAmount = TotalMoney / (1 + FeeRate)$，$Fee = TotalMoney - NetAmount$。计算过程中必须注意舍入精度，手续费通常保留两位小数，份额保留两位小数。

#### Scenario: Calculating purchase with standard formula
- **WHEN** 用户以 100 元买入净值为 2.3026 的基金，申购费率为 0.15%
- **THEN** 手续费应计算为 0.15 元（100 - 100/1.0015 ≈ 0.1497，舍入为 0.15），最终份额应为 37.77 份（(100-0.15)/2.3026 ≈ 37.7659，舍入为 37.77）。
