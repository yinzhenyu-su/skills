## 1. 数据库 Schema 扩展

- [x] 1.1 在 `db.rs` 中新增 `fund_tradability` 表：字段包含 `fund_code`（主键）、`subscription_status`、`redemption_status`、`min_subscription_amount`、`limit_per_transaction`、`settlement_days`、`last_update`
- [x] 1.2 在 `db.rs` 中新增 `redemption_fee_tiers` 表：字段包含 `id`（主键）、`fund_code`、`min_days`、`max_days`（可空，NULL 表示无上限）、`fee_rate`
- [x] 1.3 在 `db::setup` 中使用 `CREATE TABLE IF NOT EXISTS` 创建两张新表，保证向后兼容
- [x] 1.4 编写 `db::upsert_fund_tradability` 函数（INSERT OR REPLACE）
- [x] 1.5 编写 `db::replace_redemption_fee_tiers` 函数：先删除旧档位，再批量插入新记录
- [x] 1.6 编写 `db::get_redemption_fee_rate` 函数：按 `fund_code` 和 `holding_days` 查询适用费率，返回 `Option<Decimal>`
- [x] 1.7 编写 `db::get_fund_tradability` 函数：按 `fund_code` 查询单条可交易性记录

## 2. Provider 扩展（多源：lsjz + Morningstar fees）

- [x] 2.1 阅读并记录 Eastmoney `lsjz` 中可交易状态字段（`SGZT/SHZT`）以及 Morningstar `fees` 中分段费率字段（在 `src/provider/docs/` 中更新文档）
- [x] 2.2 在 `FundData` struct（`src/provider/mod.rs`）中新增字段：`subscription_status`、`redemption_status`、`min_subscription_amount`、`limit_per_transaction`、`settlement_days`
- [x] 2.3 在 `FundData` struct 中新增 `redemption_fee_tiers: Vec<(i32, Option<i32>, Decimal)>`（min_days, max_days, fee_rate）
- [x] 2.4 更新 `eastmoney_lsjz.rs`，解析 `SGZT/SHZT` 并填充 `FundData.subscription_status/redemption_status`
- [x] 2.5 更新 `morningstar.rs`，解析 `redemptionFee/frontLoadFee/deferLoadFee` 并填充 `FundData.redemption_fee_tiers`
- [x] 2.6 更新 `aggregator.rs`，在合并 `FundData` 时正确传播可交易性字段和费率数组
- [x] 2.7 增加费率单位映射：`feeUnit/floorUnit` 转换为内部统一结构（百分比/固定金额、天/月/金额阈值）
- [x] 2.8 增加 `purchaseAndRedeem.applyingMax*` 语义探针任务，完成 `limit_per_transaction` 字段映射表
- [x] 2.9 `settlement_days` 无稳定来源时写入 `NULL`，并确保命令层展示"未知"

## 3. 同步逻辑更新

- [x] 3.1 更新 `resolver::sync_fund_details`，在写入基金数据后，若 `FundData` 包含可交易性字段，则调用 `db::upsert_fund_tradability`
- [x] 3.2 更新 `resolver::sync_fund_details`，若 `FundData.redemption_fee_tiers` 非空，则调用 `db::replace_redemption_fee_tiers`
- [x] 3.3 若 `FundData` 中 `settlement_days` 缺失，保持可交易性记录其他字段可更新，不因单字段缺失中断同步

## 4. finance.rs 新增持有天数工具函数

- [x] 4.1 在 `finance.rs` 中新增 `days_since(date_str: &str) -> i64` 工具函数，计算给定日期到今天的天数
- [x] 4.2 单元测试：验证 `days_since` 在正常日期、今天、未来日期下的行为

## 5. 持有天数 + 仓位占比（db.rs 查询）

- [x] 5.1 新增 `db::get_first_buy_date(conn, wallet_id, fund_code) -> Option<String>` 查询函数，返回该基金在该钱包中最早已结算 buy/import 交易的日期
- [x] 5.2 更新 `db::get_holdings` 返回的持仓结构体，新增可选字段 `holding_days: Option<i64>` 和 `allocation_pct: Option<f64>`
- [x] 5.3 在 `main.rs` 的持仓查询逻辑中，为每条持仓记录填充 `holding_days`（调用 `get_first_buy_date` + `days_since`）和 `allocation_pct`（各基金市值 / 总市值）

## 6. status 命令展示更新

- [x] 6.1 在 `status` 命令的持仓表格中新增"持有天数"列，显示 `holding_days`（无数据显示 `-`）
- [x] 6.2 在 `status` 命令的持仓表格中新增"赎回费率"列，调用 `db::get_redemption_fee_rate` 获取当前适用档位，并按费率高低设置颜色（0% 绿色，>1% 红色）
- [x] 6.3 在 `status` 命令的持仓表格中新增"仓位占比"列，显示 `allocation_pct`（无数据显示 `-`）
- [x] 6.4 在 `status` 表格底部追加"合计"行，展示总市值、总成本、总浮盈亏金额和百分比

## 7. preview sell 命令集成分档赎回费

- [x] 7.1 在 `handle_preview_sell` 函数中，若未传入 `--fee` 参数，则查询 `db::get_first_buy_date` 得到持有天数，再调用 `db::get_redemption_fee_rate` 获取适用费率
- [x] 7.2 在 `preview sell` 输出中新增持有天数、适用费率及所在档位的描述文字
- [x] 7.3 若 `--fee` 已显式传入，在输出中标注"（手动指定）"，跳过自动查询
- [x] 7.4 若无费率数据，输出 `⚠️ 赎回费率数据不可用，以下金额仅供参考` 并使用 0.5% 默认费率

## 8. sell 命令集成分档赎回费

- [x] 8.1 在 `handle_sell` 函数中，若未传入 `--fee` 参数，则先查询持有天数和适用赎回费率，用于计算实际手续费
- [x] 8.2 在 `sell` 成功后的确认输出中展示实际使用的赎回费率及档位来源
- [x] 8.3 若无费率数据，输出 `⚠️` 提示，并继续以 0.5% 默认费率执行（不阻断流程）

## 9. inspect 命令展示可交易性信息

- [x] 9.1 在 `format_inspect_report` 中新增"交易信息"分区，展示申购状态、赎回状态、最低买入额、单笔限购额、资金到账天数
- [x] 9.2 申购/赎回状态暂停时以黄色高亮显示
- [x] 9.3 若 `fund_tradability` 无数据，显示"交易信息: 数据不可用（运行 fund sync 更新）"

## 10. 测试

- [x] 10.1 单元测试：`db::get_redemption_fee_rate` 覆盖三种场景（低档/中档/免费）
- [x] 10.2 单元测试：`db::get_first_buy_date` 覆盖单次买入、多次买入、无持仓三种场景
- [x] 10.3 单元测试：`finance::days_since` 覆盖正常日期和边界日期
- [x] 10.4 集成测试：`preview sell` 输出包含持有天数和适用赎回费率信息
- [x] 10.5 集成测试：`status` 命令输出包含"持有天数"、"赎回费率"和"仓位占比"列
- [x] 10.6 集成测试：`sell` 命令在无 `--fee` 参数时自动应用分档费率
