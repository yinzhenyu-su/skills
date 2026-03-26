## Why

`fund reset` 命令错误地放在了 `fund` 子命令下，用户期望的路径是 `fund-manager reset`（顶层命令）。同时 CLI 帮助信息中存在大量误导性示例，使用 `fund status`、`fund buy` 等错误前缀（应该是 `fund-manager status`、`fund-manager buy`）。

## What Changes

- 将 `reset` 命令从 `FundCommands` 移至 `Commands` 顶层，实现 `fund-manager reset` 路径
- 修正所有 `long_about` 示例中的错误命令前缀：`fund status` → `fund-manager status`、`fund buy` → `fund-manager buy` 等
- 改进 `reset` 命令的帮助信息，详细列举删除的数据项并强调不可恢复
- 更新 `db.rs` 注释和测试用例中的命令路径引用

## Capabilities

### New Capabilities

- `fund-reset`: 提供 `fund-manager reset` 命令，清除所有个人数据并重建空数据库

### Modified Capabilities

- `fund-reset`: 修正命令路径从 `fund fund reset` 变为 `fund-manager reset`

## Impact

- **CLI 结构**：`src/cli.rs` 的 `Commands` 和 `FundCommands` 枚举
- **命令处理**：`src/main.rs` 中 `FundCommands::Reset` 分支移至 `Commands::Reset`
- **数据库**：`src/db.rs` 注释更新
- **测试**：`tests/cli_tests.rs` 中测试命令路径更新
- **文档**：所有 OpenSpec 归档文件中的命令引用
