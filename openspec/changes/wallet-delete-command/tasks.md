## 1. CLI 配置与本地化

- [x] 1.1 在 `src/cli.rs` 中完善 `WalletCommands::Delete` 的定义，添加 `del` 别名，并将文档说明翻译为中文。

## 2. 数据库逻辑实现

- [x] 2.1 在 `src/db.rs` 中实现 `delete_wallet(conn, wallet_id)` 函数，逻辑应包含：
  - 检查该 ID 是否为当前活跃钱包。
  - 如果是，则从 `app_config` 中移除 `active_wallet_id`。
  - 执行 `DELETE FROM wallet WHERE id = ?` 语句。
- [x] 2.2 移除原有的 `delete_wallet_by_name` 或 `clear_active_wallet` 等冗余函数。

## 3. 业务逻辑与错误处理

- [x] 3.1 在 `src/main.rs` 中更新 `WalletCommands::Delete` 的路由逻辑。
- [x] 3.2 调用 `db::get_wallet_id_by_name` 查找 ID，并 handle 钱包不存在的错误（输出中文提示）。
- [x] 3.3 使用 `confirm_action` 实现中文交互确认提示。
- [x] 3.4 移除代码中的 `expect` 和 `unwrap`，统一使用更健壮的错误处理。

## 4. 测试验证

- [x] 4.1 在 `src/db.rs` 中添加单元测试，验证删除钱包、清理活跃配置以及交易记录的级联删除。
- [x] 4.2 添加集成测试，验证 `fund wallet delete` 命令的完整流程。
- [x] 4.3 运行 `cargo test` 确保所有测试通过且无回归。
