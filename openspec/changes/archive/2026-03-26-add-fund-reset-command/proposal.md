## Why

用户在使用 fund-manager 一段时间后，可能需要重新整理投资组合或清除所有数据从头开始。目前没有一键清除所有个人数据的命令，用户只能逐个删除钱包和基金，操作繁琐且容易遗漏某些数据。

## What Changes

- 新增 `fund reset` 命令，一键清除所有个人数据（钱包、基金、交易历史、配置），保留数据库表结构
- 操作不可逆，执行前需用户确认（可跳过确认）
- 清除范围：wallet、transaction_log、fund、nav_history、fund_analysis、fund_tradability、redemption_fee_tiers、app_config

## Capabilities

### New Capabilities

- `fund-reset`: 提供 `fund reset` 命令，清除所有个人数据并重建空数据库

## Impact

- **核心影响**：`src/main.rs` 新增 `Commands::Reset` 分支
- **数据库**：`db.rs` 新增 `reset_all_data()` 函数
- **CLI 参数**：`src/cli.rs` 新增 `Reset` 命令定义
