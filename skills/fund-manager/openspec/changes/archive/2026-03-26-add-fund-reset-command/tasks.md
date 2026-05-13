## 1. CLI 定义

- [x] 1.1 在 `src/cli.rs` 的 `Commands` 枚举中添加 `Reset` 变体
- [x] 1.2 为 `Reset` 添加命令描述和使用示例

## 2. 数据库操作

- [x] 2.1 在 `src/db.rs` 中添加 `reset_all_data(conn: &Connection, db_path: &Path) -> Result<()>`
- [x] 2.2 实现：关闭连接、删除数据库文件、调用 `init_db` 重建 schema

## 3. 命令处理

- [x] 3.1 在 `src/main.rs` 添加 `Commands::Reset` 分支
- [x] 3.2 调用 `confirm_action` 显示危险操作警告
- [x] 3.3 执行 `db::reset_all_data()`
- [x] 3.4 打印成功提示并以退出码 0 退出

## 4. 测试

- [x] 4.1 添加 CLI 集成测试：验证 `fund-manager reset -y` 成功重置数据
