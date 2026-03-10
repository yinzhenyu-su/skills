## 1. CLI 定义更新

- [x] 1.1 在 `src/cli.rs` 的 `Cli` 结构体中添加 `yes` 字段，标记为 `short = 'y', long = "yes", global = true`。

## 2. 确认逻辑抽象 (Infrastructure)

- [x] 2.1 在 `src/main.rs` 或新模块中实现 `confirm_action(prompt: &str, force_yes: bool) -> bool` 工具函数。
- [x] 2.2 确保该函数在 `force_yes` 为真时打印跳过提示（如：`"Skipping confirmation (-y detected)..."`）。

## 3. 集成与替换 (Implementation)

- [x] 3.1 替换 `fund fund delete` 中的手动确认逻辑，改为使用 `confirm_action`。
- [x] 3.2 替换 `fund sell` 中的预览确认逻辑，改为使用 `confirm_action`（配合 `refactor-sell-command` 任务）。

## 4. 验证与集成测试

- [x] 4.1 编写集成测试，验证 `fund fund delete -y` 能够在非交互模式下成功删除数据。
- [x] 4.2 验证在不加 `-y` 时，原有的交互式提示依然有效且能正确处理 `n` 信号。
