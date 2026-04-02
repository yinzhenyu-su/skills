## 1. 数据库与 CLI 基础架构

- [x] 1.1 在 `src/db.rs` 的 `fund` 表中新增 `target_profit_rate` 和 `stop_loss_rate` 字段
- [x] 1.2 在 `src/cli.rs` 中为 `fund config` 命令添加 `--target-profit` 和 `--stop-loss` 可选参数
- [x] 1.3 在 `src/main.rs` 中实现 `fund config` 的持久化逻辑

## 2. 核心逻辑计算

- [x] 2.1 在 `src/finance.rs` 中实现保本净值计算函数 `calculate_breakeven_nav`
- [x] 2.2 在 `src/db.rs` 中实现 `get_next_redemption_fee_tier` 函数，用于查找下一跳档日期和费率
- [x] 2.3 在 `src/finance.rs` 中实现阶梯跳档检测逻辑，返回剩余天数和预估节省金额

## 3. UI 展示与集成

- [x] 3.1 修改 `src/main.rs` 的 `status` 命令，增加“保本净值”和“提醒”列
- [x] 3.2 在 `status` 表格渲染中集成止盈止损报警（🎯/⚠️ 符号及颜色高亮）
- [x] 3.3 在 `status` 表格底部增加汇总决策建议文案
- [x] 3.4 修改 `preview sell` 命令，在输出结尾增加持有期优化建议 Block

## 4. 验证与测试

- [x] 4.1 为保本净值计算编写单元测试
- [x] 4.2 为费率跳档检测逻辑编写单元测试
- [x] 4.3 验证 CLI 输出在不同场景下的展示效果
