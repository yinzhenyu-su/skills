## MODIFIED Requirements

### Requirement: Sell Transaction Pre-check
在执行 `sell` 命令前，系统 SHALL 检查指定钱包在指定基金中的可用持有量。如果余额不足，SHALL 阻止交易并提供具体的可行性建议。

#### Scenario: 卖出份额超出持有量
- **WHEN** 钱包 "Default" 持有 500 份 "000300"，但执行 `fund sell 000300 --shares 1000`
- **THEN** 系统 SHALL 报错：`❌ 卖出失败：你在 [Default] 中仅持有 500.00 份 '000300'，无法卖出 1000 份。`
