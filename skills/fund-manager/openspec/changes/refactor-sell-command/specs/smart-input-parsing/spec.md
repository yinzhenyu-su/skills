## ADDED Requirements

### Requirement: Fractional Share Resolution
系统 SHALL 支持解析分数形式的份额输入。

#### Scenario: Parsing 1/2 fraction
- **GIVEN** 当前持有 1000 份
- **WHEN** 输入份额为 "1/2"
- **THEN** 系统解析结果为 500.00 份。

### Requirement: Mixed Fee Calculation
系统 SHALL 支持解析固定金额或百分比费率。

#### Scenario: Parsing percentage fee
- **GIVEN** 赎回总额为 1000.00
- **WHEN** 手续费输入为 "0.5%"
- **THEN** 系统计算手续费为 5.00。

#### Scenario: Parsing fixed fee
- **WHEN** 手续费输入为 "5.0"
- **THEN** 系统解析手续费为 5.00。
