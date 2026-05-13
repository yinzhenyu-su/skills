## 1. CLI 结构修改（cli.rs）

- [x] 1.1 新增 `PreviewCommands` 枚举，包含 `Buy` 和 `Sell` 变体（参数与现有 `PreviewBuy`/`PreviewSell` 相同）
- [x] 1.2 将 `Commands::PreviewBuy` 和 `Commands::PreviewSell` 替换为单一的 `Commands::Preview { command: PreviewCommands }`
- [x] 1.3 更新 `Preview` 变体的 `long_about` 帮助示例，改为正确格式（`fund-manager preview buy 000312 --money 5000`）

## 2. 命令分发修改（main.rs）

- [x] 2.1 将 `Commands::PreviewBuy { .. }` 和 `Commands::PreviewSell { .. }` 的匹配分支替换为嵌套的 `Commands::Preview { command }` + `PreviewCommands::Buy/Sell` 匹配

## 3. 验证与测试

- [x] 3.1 运行 `cargo build` 确认编译通过，无新增 warnings
- [x] 3.2 验证 `fund-manager preview buy --help` 输出正确帮助文本
- [x] 3.3 验证 `fund-manager preview sell --help` 输出正确帮助文本
- [x] 3.4 验证 `fund-manager preview --help` 展示 `buy` 和 `sell` 子命令列表
- [x] 3.5 验证 `fund-manager preview-sell` 返回 "unrecognized subcommand" 错误（breaking change 确认）
- [x] 3.6 运行 `cargo test` 确认所有现有测试通过

## 4. 后续优化（实现过程中追加）

- [x] 4.1 将 `PreviewCommands::Buy/Sell` 的 `fund` 字段改为 `Option<String>`，避免 clap 原始错误信息暴露给用户
- [x] 4.2 新增 `resolve_fund_interactively(conn, fund, wallet_id, subcommand)` 辅助函数：缺少 `<FUND>` 时打印已追踪基金列表并附用法示例，以 `exit(1)` 退出
- [x] 4.3 修改 `smart_nav_lookup` 回退策略：优先向后查找最近30天净值（`find_prev_available_nav`），再向前查找20天（`find_next_available_nav`），适配 QDII T+1/T+2 延迟场景
- [x] 4.4 优化 `handle_preview_buy` 缺参错误信息：改为多行说明，列出 `--money` 和 `--shares` 选项及示例
- [x] 4.5 优化 `handle_preview_sell` 缺参错误信息：额外展示 `--shares all` 和 `--shares 1/2` 特殊语法示例
- [x] 4.6 修复 `HoldingImportItem.line_num` 字段的 `dead_code` 编译警告（添加 `#[allow(dead_code)]`）
