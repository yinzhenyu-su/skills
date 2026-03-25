## ADDED Requirements

### Requirement: 钱包内基金仓位占比计算

系统 SHALL 计算每只基金的当前市值占当前钱包全部持仓总市值的百分比，结果在 `status` 命令中展示。

#### Scenario: 两只基金的仓位占比

- **WHEN** 钱包持有基金 A 市值 6000 元、基金 B 市值 4000 元，总市值 10000 元
- **THEN** `status` 输出中基金 A 的仓位占比 SHALL 显示 `60.00%`，基金 B 显示 `40.00%`

#### Scenario: 仅持有一只基金

- **WHEN** 钱包只持有一只基金
- **THEN** 该基金仓位占比 SHALL 显示 `100.00%`

#### Scenario: 净值缺失时的仓位占比

- **WHEN** 某基金无最新净值数据，无法计算市值
- **THEN** 仓位占比列 SHALL 显示 `-`，不计入分母

#### Scenario: 无持仓时不展示占比列

- **WHEN** 当前钱包无任何持仓
- **THEN** `status` 输出 SHALL 不展示占比列，正常显示无持仓提示

### Requirement: status 命令底部汇总总仓位

`status` 命令 SHALL 在表格底部追加一行"合计"，展示钱包总市值、总成本和总盈亏。

#### Scenario: 展示汇总行

- **WHEN** 用户运行 `fund-manager status` 且持有多只基金
- **THEN** 表格最后一行 SHALL 为"合计"，显示总市值之和、总成本之和、总浮盈亏金额及百分比
