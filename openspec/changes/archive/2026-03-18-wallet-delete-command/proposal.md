## Why

当前用户无法删除不再需要的钱包，导致投资组合管理不够灵活。为对齐 `fund delete` 的用户体验，需要为 `wallet` 命令添加相应的删除功能。此外，该功能的当前实现仍处于英文状态，需要进行全中文本地化。

## What Changes

- **新增子命令**：为 `wallet` 命令添加 `delete` 子命令（别名 `del`），用于彻底删除钱包。
- **级联删除**：实现钱包删除时的数据库级联操作，自动清理关联的所有交易记录。
- **活跃钱包状态维护**：如果被删除的是当前活跃钱包，需自动清除 `app_config` 中的选中状态。
- **风险提示与确认**：增加交互式确认提示，明确告知删除操作的不可逆性及其带来的数据影响，并支持全局 `-y` 参数跳过。
- **全中文本地化**：将所有新增的文档说明、提示语和成功消息转换为中文。

## Capabilities

### New Capabilities
- `wallet-deletion`: 提供彻底移除钱包及其所有历史数据的能力。

### Modified Capabilities
- `chinese-localization`: 将新增的钱包管理命令及其交互反馈纳入中文本地化覆盖范围。
- `confirm-bypass`: 确保钱包删除操作遵循现有的全局确认绕过逻辑。

## Impact

- `skills/fund-manager/src/cli.rs`: 扩展 `WalletCommands` 枚举。
- `skills/fund-manager/src/db.rs`: 增加 `delete_wallet_by_name` 和 `clear_active_wallet` 等辅助函数。
- `skills/fund-manager/src/main.rs`: 实现 `WalletCommands::Delete` 的路由逻辑。
- 数据库完整性：依赖 `transaction_log` 对 `wallet_id` 的 `ON DELETE CASCADE` 约束。
