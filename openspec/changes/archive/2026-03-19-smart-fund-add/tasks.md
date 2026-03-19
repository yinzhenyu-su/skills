## 1. CLI 参数调整

- [x] 1.1 修改 `src/cli.rs` 中 `FundCommands::Add` 的定义，将 `code` 和 `name` 移除，替换为 `fund: Option<String>`
- [x] 1.2 将 `FundCommands::Add` 的 `fee` 类型从 `String` 改为 `Option<String>`
- [x] 1.3 更新 `FundCommands::Add` 的文档注释，符合 `require_fund_or_exit` 的约定

## 2. Handler 逻辑重构

- [x] 2.1 修改 `src/main.rs` 中的 `FundCommands::Add` 的匹配分支以适配新参数
- [x] 2.2 在处理 `fund add` 时调用 `require_fund_or_exit` 拦截参数缺失
- [x] 2.3 使用 `resolver::resolve_fund(&conn, &fund_input, false).await` 获取基金对象
- [x] 2.4 调用 `db::add_fund` 使用提取出的 `code`, `name`, `fee` 存入数据库
- [x] 2.5 确保原有错误被捕获并使用自定义错误格式打印

## 3. 测试与兼容性修复

- [x] 3.1 搜索并替换 `tests/cli_tests.rs` 中所有旧的 `fund add <code> <name>` 调用，改为 `fund add <code>`
- [x] 3.2 搜索并替换 `tests/cli_wallet_tests.rs` 中所有旧的 `fund add` 调用
- [x] 3.3 补充对 `fund add` 参数缺失场景的报错断言测试
- [x] 3.4 运行 `cargo test` 验证通过
