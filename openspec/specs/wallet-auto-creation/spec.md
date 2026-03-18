## ADDED Requirements

### Requirement: 自动创建默认钱包

当用户执行需要钱包的命令（如 `buy`、`sell`、`status`）且没有活跃钱包时，系统 SHALL 自动创建一个名为"默认钱包"的新钱包并将其设置为活跃钱包。

#### Scenario: 首次使用自动创建钱包
- **WHEN** 用户未创建过任何钱包，执行 `fund buy 000300 --money 1000`
- **THEN** 系统 SHALL 自动创建名为"默认钱包"的新钱包
- **AND** 系统 SHALL 将该钱包设置为活跃钱包
- **AND** 系统 SHALL 使用该钱包完成买入操作

#### Scenario: 有其他钱包但未激活时不自动创建
- **WHEN** 用户有多个钱包但没有设置活跃钱包，执行 `fund buy 000300 --money 1000 --wallet 其他钱包`
- **THEN** 系统 SHALL 使用指定的钱包完成操作
- **AND** 系统 SHALL NOT 自动创建新钱包

### Requirement: 自动创建钱包的提示信息

当系统自动创建钱包时，系统 SHALL 向用户显示提示信息。

#### Scenario: 自动创建钱包时显示提示
- **WHEN** 系统自动创建"默认钱包"
- **THEN** 系统 SHALL 打印提示：`🔔 未检测到活跃钱包，已自动创建并激活"默认钱包"。`
