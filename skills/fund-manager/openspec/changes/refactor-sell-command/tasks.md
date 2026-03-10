## 1. CLI 参数重构

- [x] 1.1 修改 `cli::Cli` 中的 `Sell` 变体，将 `shares`, `fee`, `money`, `nav` 均改为 `Option<String>`。
- [x] 1.2 编写针对新参数结构的 `clap` 帮助文档验证测试。

## 2. 智能解析器实现 (TDD: smart-input-parsing)

- [x] 2.1 **[RED]** 编写解析分数份额（如 "1/2", "all"）的单元测试。
- [x] 2.2 **[GREEN]** 在 `finance.rs` 中实现 `resolve_shares(input: &str, total: Decimal)` 逻辑。
- [x] 2.3 **[RED]** 编写解析混合费率（如 "0.5%", "5.0"）的单元测试。
- [x] 2.4 **[GREEN]** 在 `finance.rs` 中实现 `resolve_fee(input: &str, total_money: Decimal)` 逻辑。

## 3. 卖出逻辑重构 (TDD: transaction-tracking)

- [x] 3.1 **[RED]** 编写集成测试，模拟赎回预览与确认流。
- [x] 3.2 **[GREEN]** 重构 `main.rs` 中的 `Sell` 处理逻辑：
    - 自动补全 `nav`（若缺失）。
    - 调用智能解析器确定最终 `shares` 和 `fee`。
    - 渲染预览清单并等待 `y` 确认。
- [x] 3.3 优化错误提示：卖出非追踪基金时给出“从未拥有过”的明确信息。

## 4. 回归验证与优化

- [x] 4.1 确保全量赎回 (`--shares all`) 后，`status` 中的该基金份额准确归零。
- [x] 4.2 运行所有集成测试，确保 `buy` 命令和其他功能无回归。
