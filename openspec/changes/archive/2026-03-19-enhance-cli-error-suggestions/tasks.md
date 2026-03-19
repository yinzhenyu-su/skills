## 1. 基础架构与错误拦截

- [x] 1.1 在 `main.rs` 中将 `Cli::parse()` 替换为 `Cli::try_parse()`
- [x] 1.2 在 `main.rs` 中实现初步的 `match cli_res` 错误拦截逻辑
- [x] 1.3 定义统一的 `handle_clap_error` 函数框架

## 2. AdviceEngine 智能建议扩展

- [x] 2.1 在 `resolver.rs` 的 `AdviceEngine` 中添加 `find_closest_command` 辅助方法
- [x] 2.2 实现 `check_unexpected_arg_intent` 逻辑，识别错位的子命令
- [x] 2.3 在 `AdviceEngine` 中集成 Levenshtein 模糊匹配逻辑（可选，或简单的包含关系匹配）

## 3. 错误信息本地化与格式化

- [x] 3.1 实现 `format_clap_error` 函数，处理 `UnknownArgument` 错误类型并注入建议
- [x] 3.2 实现 `MissingRequiredArgument` 的本地化输出（针对 `<FUND>`、`<WALLET>` 等）
- [x] 3.3 实现 `NoSubcommand` 的本地化输出，引导用户查看帮助
- [x] 3.4 确保所有解析错误都遵循 `❌` -> `💡` -> `   用法示例` 的统一格式

## 4. 测试与验证

- [x] 4.1 更新 `tests/cli_tests.rs` 中相关的错误匹配断言
- [x] 4.2 为 `wallet list use` 场景添加专门的集成测试用例
- [x] 4.3 运行 `cargo test` 确保所有现有功能不受影响
