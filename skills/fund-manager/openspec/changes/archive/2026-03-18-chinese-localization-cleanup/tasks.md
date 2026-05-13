## 1. 深度汉化与补全

- [x] 1.1 汉化 `main.rs` 中 `resolve_wallet_id` 的报错信息。
- [x] 1.2 汉化 `main.rs` 中 `inspect` 命令的投资者获得感解释文案。
- [x] 1.3 汉化 `main.rs` 中 `handle_import` 的 CSV 解析错误提示。
- [x] 1.4 汉化 `main.rs` 中 `Sync completed` 等收尾日志。
- [x] 1.5 汉化 `sync.rs` 中针对结算和数据库查找的剩余英文提示。

## 2. 交互细节精修

- [x] 2.1 汉化赎回操作预览中的 `PREVIEW` 字样。
- [x] 2.2 汉化交易历史命令中的 `to be implemented` 占位符。
- [x] 2.3 统一 `sync.rs` 中的日期同步提示风格。

## 3. 测试适配与最终验证

- [x] 3.1 检查 `tests/cli_tests.rs` 是否有遗漏的英文断言并更新。
- [x] 3.2 运行 `cargo test` 确保所有本地化修改不破坏现有功能。
- [x] 3.3 手动运行 `fund import` 模拟失败场景，验证中文报错输出。
