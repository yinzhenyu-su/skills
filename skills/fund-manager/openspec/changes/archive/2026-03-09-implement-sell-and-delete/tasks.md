## 1. 卖出功能 (TDD: transaction-tracking)

- [x] 1.1 **[RED]** 编写集成测试，验证 `fund sell` 减少持仓份额并记录流水。
- [x] 1.2 **[RED]** 编写测试验证超额卖出时系统应报错。
- [x] 1.3 **[GREEN]** 在 `db.rs` 中实现查询当前基金持仓份额的逻辑。
- [x] 1.4 **[GREEN]** 实现 `sell` 命令逻辑，包含份额校验。
- [x] 1.5 **[RED]** 编写 `sell --auto` 测试，根据金额反算卖出份额。
- [x] 1.6 **[GREEN]** 实现 `sell --auto` 计算逻辑。

## 2. 基金删除功能 (TDD: fund-deletion)

- [x] 2.1 **[REFACTOR]** 修改数据库初始化脚本，为 `nav_history` 和 `transaction_log` 的外键添加 `ON DELETE CASCADE`。
- [x] 2.2 **[RED]** 编写集成测试，验证 `fund delete` 后相关流水和净值历史被级联清除。
- [x] 2.3 **[GREEN]** 实现 `fund delete` 命令，并集成 `AskUserQuestion` 或类似的交互式确认逻辑（CLI 层面）。

## 3. 验证与集成

- [x] 3.1 运行所有测试确保无回归（特别是 `fund status` 的盈亏计算）。
- [x] 3.2 编写 E2E 测试：买入 -> 卖出部分 -> 查看状态 -> 删除基金 -> 确认数据库清空。
