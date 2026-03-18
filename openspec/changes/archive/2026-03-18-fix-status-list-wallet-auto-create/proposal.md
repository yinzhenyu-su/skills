## Why

`status` 和 `list` 命令缺少 `--wallet` 参数支持，且 `status` 在无活跃钱包时会 panic 而非自动创建默认钱包。这与 `buy`、`sell`、`import` 命令的行为不一致，用户体验不统一。

## What Changes

- **Status 命令**：添加 `--wallet` 可选参数，使用 `resolve_wallet_id()` 获取钱包 ID
- **List 命令**：添加 `--wallet` 可选参数，使用 `resolve_wallet_id()` 获取钱包 ID
- **移除 `.expect()` panic**：将 `status` 命令中的 `get_active_wallet_id().expect().expect()` 改为调用 `resolve_wallet_id()`
- **新增交互式选择**：有钱包但无活跃钱包时，询问用户选择（替代自动激活）

## Capabilities

### Modified Capabilities

- `wallet-auto-creation`：现有 spec 已定义 `status` 命令应支持自动创建钱包，但实现不完整。本次修改完成该 spec 的实现。

## Impact

- 修改 `src/cli.rs`：给 `Status` 命令结构体添加 `wallet: Option<String>` 字段
- 修改 `src/main.rs`：`Commands::Status` 分支改用 `resolve_wallet_id(&conn, wallet)`
- 修改 `src/main.rs`：`FundCommands::List` 分支添加 `wallet: Option<String>` 参数并改用 `resolve_wallet_id()`
