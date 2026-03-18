## 1. 修复外键约束问题

- [x] 1.1 在 `db.rs` 中创建 `open_conn(path: P) -> Connection` 函数，统一在打开连接后执行 `PRAGMA foreign_keys = ON`
- [x] 1.2 修改 `main.rs` 中所有 `Connection::open()` 调用为 `open_conn()`
- [x] 1.3 验证：编写测试确认删除钱包时 `transaction_log` 记录被级联删除（现有 test_delete_wallet 测试通过）

## 2. 添加钱包自动创建功能

- [x] 2.1 修改 `resolve_wallet_id()` 函数，当没有活跃钱包时自动创建"默认钱包"并激活
- [x] 2.2 在自动创建钱包后打印提示信息：`🔔 未检测到活跃钱包，已自动创建并激活「默认钱包」。`
- [x] 2.3 编写集成测试验证自动创建行为（通过 CLI 测试验证）

## 3. 添加钱包重命名功能

- [x] 3.1 在 `cli.rs` 的 `WalletCommands` 中添加 `Rename { old_name: String, new_name: String }` 变体
- [x] 3.2 在 `db.rs` 中添加 `rename_wallet(conn: &Connection, old_name: &str, new_name: &str) -> Result<()>`
- [x] 3.3 在 `main.rs` 中实现 `WalletCommands::Rename` 分支处理逻辑
- [x] 3.4 编写测试验证重命名功能（4 个测试：成功重命名、重命名冲突、重命名不存在、改名后保持活跃）

## 4. 测试验证

- [x] 4.1 运行 `cargo test` 确保所有现有测试通过（73 tests）
- [x] 4.2 添加钱包重命名相关的单元测试（4 个新测试）
- [ ] 4.3 手动测试完整流程：创建钱包 → 交易 → 重命名 → 删除
