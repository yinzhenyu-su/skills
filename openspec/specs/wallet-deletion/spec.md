## ADDED Requirements

### Requirement: 钱包删除命令
系统 SHALL 提供 `fund wallet delete <名称>` 命令，并支持别名 `del`，用于从数据库中移除指定的钱包。

#### Scenario: 成功删除钱包
- **WHEN** 用户执行 `fund wallet delete 我的投资`
- **THEN** 系统 SHALL 从 `wallet` 表中删除名称为 "我的投资" 的记录。
- **AND** 由于级联删除约束，`transaction_log` 中所有关联该钱包的交易记录 SHALL 被自动移除。

### Requirement: 活跃钱包状态维护
如果被删除的钱包是当前 `app_config` 中记录的 `active_wallet_id`，系统 SHALL 清除该配置。

#### Scenario: 删除当前活跃钱包
- **WHEN** 钱包 "我的投资" 是当前活跃钱包，且用户执行删除该钱包的命令
- **THEN** 系统 SHALL 在成功删除钱包后，从 `app_config` 中移除 `active_wallet_id` 的键值对。
- **AND** 提示用户：`✅ 钱包 '我的投资' 已成功删除。(由于原为活跃钱包，当前未选中任何钱包。)`

### Requirement: 钱包删除的交互式确认
在执行删除操作前，系统 SHALL 向用户展示风险提示并要求输入 "y" 确认。

#### Scenario: 删除前的风险提示
- **WHEN** 用户执行 `fund wallet delete 我的投资`
- **THEN** 系统 SHALL 打印提示：`确定要删除钱包 '我的投资' 吗？这将永久删除其所有的交易记录！ [y/N]: `
- **AND** 仅当用户输入 "y" 或 "Y" 时才继续操作。
