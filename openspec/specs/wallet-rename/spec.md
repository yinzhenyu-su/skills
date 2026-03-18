## ADDED Requirements

### Requirement: 钱包重命名命令

系统 SHALL 提供 `fund wallet rename <旧名> <新名>` 命令，用于对已有钱包进行重命名。

#### Scenario: 成功重命名钱包
- **WHEN** 用户执行 `fund wallet rename 我的投资 投资组合`
- **THEN** 系统 SHALL 将名称为"我的投资"的钱包重命名为"投资组合"
- **AND** 系统 SHALL 打印成功消息：`✅ 钱包已从"我的投资"重命名为"投资组合"。`

#### Scenario: 重命名为已存在的钱包名
- **WHEN** 用户执行 `fund wallet rename 钱包A 钱包B`，但"钱包B"已存在
- **THEN** 系统 SHALL 打印错误消息：`❌ 错误：钱包"钱包B"已存在。`
- **AND** 系统 SHALL NOT 执行重命名操作

#### Scenario: 重命名不存在钱包
- **WHEN** 用户执行 `fund wallet rename 不存在的钱包 新名字`
- **THEN** 系统 SHALL 打印错误消息：`❌ 错误：找不到名为"不存在的钱包"的钱包。`

#### Scenario: 重命名当前活跃钱包
- **WHEN** 用户重命名当前活跃钱包
- **THEN** 系统 SHALL 在重命名成功后保持该钱包为活跃状态
