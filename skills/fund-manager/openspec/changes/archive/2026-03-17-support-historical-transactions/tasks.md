## 1. CLI 接口扩展

- [x] 1.1 在 `src/cli.rs` 的 `Buy` 子命令增加 `date: Option<String>` 参数。
- [x] 1.2 在 `src/cli.rs` 的 `Sell` 子命令增加 `date: Option<String>` 参数。
- [x] 1.3 在 `src/cli.rs` 的 `Import` 子命令增加 `date: Option<String>` 参数。

## 2. 核心逻辑重构

- [x] 2.1 修改 `src/main.rs` 中的 `handle_import` 函数，增加对可选日期列的解析逻辑。
- [x] 2.2 修改 `src/main.rs` 的 `Buy` 命令逻辑，将硬编码的 `today` 替换为解析后的交易日期。
- [x] 2.3 修改 `src/main.rs` 的 `Sell` 命令逻辑，支持使用指定的交易日期进行记录。
- [x] 2.4 在 `main.rs` 中添加日期格式合法性校验函数（YYYY-MM-DD）。

## 3. 集成测试重构

- [x] 3.1 修改 `tests/cli_tests.rs`，所有涉及 `Buy/Sell/Import` 的测试显式传入固定的历史日期。
- [x] 3.2 移除测试中对 `chrono::Local::now()` 的依赖，以及相关的 `SKIP_SYNC` 等权宜之计。
- [x] 3.3 运行 `cargo test` 验证所有测试获得通过。
