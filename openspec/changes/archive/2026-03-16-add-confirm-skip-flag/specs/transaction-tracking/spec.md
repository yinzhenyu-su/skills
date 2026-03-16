## MODIFIED Requirements

### Requirement: Sell Transaction Support
系统必须支持记录“卖出”类型的交易，并提供直观的结算预览，除非显式指定了跳过标志。

#### Scenario: Selling with interactive confirmation
- **WHEN** 用户执行 `fund sell 000300 --shares 1/2`
- **THEN** 系统显示包含卖出份额、最新净值、预估金额和手续费的清单，并提示：确认记录此笔交易？ [y/N]
- **AND** 当用户输入 "y" 时，才正式写入数据库。

#### Scenario: Selling with confirmation bypass
- **WHEN** 用户执行 `fund sell 000300 --shares 1/2 -y`
- **THEN** 系统直接写入数据库并打印操作总结，不再显示交互式提示。
