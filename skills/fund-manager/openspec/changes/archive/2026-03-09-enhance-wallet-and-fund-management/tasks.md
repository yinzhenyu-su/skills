## 1. 基金列表与钱包汇总 (wallet-analytics & fund-list)

- [x] 1.1 实现 `db::get_all_funds` 并在 `fund fund list` 中展示。
- [x] 1.2 在 `db.rs` 中增加获取所有钱包及其对应 ID 的方法。
- [x] 1.3 增强 `fund wallet list`：对每个钱包调用盈亏计算逻辑并以表格形式输出分析结果。
- [x] 1.4 在 `wallet list` 中突出显示（如 `*`）当前的活跃钱包。

## 2. 交易参数增强 (Context Override)

- [x] 2.1 修改 `cli::Cli` 结构，为 `Buy` 和 `Sell` 变体增加可选的 `wallet` 字段。
- [x] 2.2 在 `main.rs` 的交易处理中引入钱包解析优先级逻辑（命令行 > 数据库活跃）。
- [x] 2.3 编写测试验证通过 `--wallet` 参数在非活跃钱包中成功记录交易。

## 3. 智能基金发现逻辑 (fund-discovery)

- [x] 3.1 在 `main.rs` 交易逻辑前插入“代码合法性检查”与“自动创建”逻辑。
- [x] 3.2 联调 `Aggregator`：如果 `buy` 命令指定的代码不存在，则自动抓取并 `add_fund`。
- [x] 3.3 实现 `sell` 命令的严格存在性校验，若本地数据库没有该基金记录则报错。

## 4. 全流程验证

- [x] 4.1 编写 E2E 测试：在没有预先添加基金的情况下，直接对指定非活跃钱包执行 `buy --auto`。
- [x] 4.2 验证 `wallet list` 在多笔异动后的数据准确性。
