## ADDED Requirements

### Requirement: 保本净值计算
系统 SHALL 根据当前持仓的加权平均成本、持有份额以及当前适用赎回费率，计算“卖出回本”所需的最低净值。

公式：`Breakeven_NAV = Total_Net_Cost / (Total_Shares * (1 - Redemption_Fee_Rate))`

#### Scenario: 计算简单保本价
- **WHEN** 某基金总持仓成本 1000 元，持有 1000 份，当前赎回费率为 0.5%
- **THEN** 保本净值 SHALL 为 `1000 / (1000 * 0.995) ≈ 1.0050`

#### Scenario: 零费率时的保本价
- **WHEN** 某基金赎回费率为 0%
- **THEN** 保本净值 SHALL 等于单位成本 `1000 / 1000 = 1.0000`

### Requirement: 保本净值展示
`status` 命令 SHALL 在持仓列表中新增“保本净值”列，展示上述计算结果。

#### Scenario: 在 status 列表中展示保本价
- **WHEN** 用户运行 `fund-manager status`
- **THEN** 表格中 SHALL 包含“保本净值”列，并显示每只基金对应的数值
