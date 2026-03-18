## Why

当前钱包管理存在三个问题：

1. **外键约束未生效**：数据库迁移时正确设置了 `PRAGMA foreign_keys = ON`，但之后打开的新连接未重新设置，导致删除钱包时 `transaction_log` 的级联删除未生效，遗留孤立的交易记录。

2. **缺少自动创建钱包逻辑**：当用户未创建过钱包或未激活任何钱包时，执行 `fund buy`、`fund sell` 等命令会直接报错退出。用户体验不流畅。

3. **缺少钱包重命名功能**：用户无法对已有钱包进行重命名操作。

## What Changes

- **Bug Fix**: 修复外键约束未在运行时生效的问题，确保钱包删除时关联数据同步清理。
- **New Feature**: 当没有活跃钱包时，自动创建名为"默认钱包"的新钱包并激活。
- **New Feature**: 添加 `fund wallet rename <旧名> <新名>` 命令。

## Capabilities

### New Capabilities

- `wallet-auto-creation`: 当没有活跃钱包时，系统自动创建并激活一个名为"默认钱包"的钱包
- `wallet-rename`: 支持对已有钱包进行重命名

### Modified Capabilities

- (无 spec 级别变更 - 修复的是实现 Bug)

## Impact

- `src/db.rs`: 连接打开后需设置 `PRAGMA foreign_keys = ON`
- `src/main.rs`: `resolve_wallet_id` 函数增加自动创建逻辑
- `src/cli.rs`: 增加 `Rename` 子命令
- `tests/`: 补充相关测试用例
