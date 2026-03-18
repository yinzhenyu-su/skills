## MODIFIED Requirements

### Requirement: 钱包自动解析逻辑

当用户执行需要钱包的命令（如 `buy`、`sell`、`import`、`status`、`list`）时，系统 SHALL 按以下优先级解析钱包：

1. **有指定钱包参数** → 使用指定钱包
2. **无指定参数 + 有活跃钱包** → 使用活跃钱包
3. **无指定参数 + 无活跃钱包 + 有其他钱包** → 交互式询问用户选择
4. **无指定参数 + 无活跃钱包 + 没有任何钱包** → 自动创建"默认钱包"并设为活跃

#### Scenario: 首次使用（无任何钱包）自动创建默认钱包
- **WHEN** 用户未创建过任何钱包，执行 `fund status`
- **THEN** 系统 SHALL 自动创建名为"默认钱包"的新钱包
- **AND** 系统 SHALL 将该钱包设置为活跃钱包
- **AND** 系统 SHALL 显示钱包内所有基金的持仓状态

#### Scenario: 有其他钱包但未激活时询问选择
- **WHEN** 用户有多个钱包但没有设置活跃钱包，执行 `fund status`
- **THEN** 系统 SHALL 显示所有钱包列表供用户选择
- **AND** 系统 SHALL 将用户选择的钱包设为活跃钱包
- **AND** 系统 SHALL 使用该钱包完成操作

#### Scenario: 指定钱包参数时直接使用
- **WHEN** 用户有多个钱包但没有设置活跃钱包，执行 `fund status --wallet 其他钱包`
- **THEN** 系统 SHALL 使用指定的钱包完成操作
- **AND** 系统 SHALL NOT 自动创建新钱包
- **AND** 系统 SHALL NOT 询问用户选择
