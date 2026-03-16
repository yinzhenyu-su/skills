## 1. 统一 HTTP 客户端

- [x] 1.1 在 `src/config.rs` 中增加默认 UA 配置并提供读取接口。
- [x] 1.2 在 `src/provider/mod.rs` 中实现 `get_client()` 工具函数，注入 UA 和 Referer。
- [x] 1.3 重构所有现有 Provider (`eastmoney_js.rs`, `eastmoney_details.rs`, `eastmoney_html.rs`, `ths_search.rs`) 使用统一 Client。

## 2. 数据库迁移与模型更新

- [x] 2.1 在 `src/db.rs` 的 `init_db` 中增加对 `transaction_log` 表的迁移逻辑（`shares`, `nav` 设为 Nullable，增加 `status`）。
- [x] 2.2 更新 `add_transaction` 函数以支持 `Option` 类型的 `shares` 和 `nav` 以及状态参数。
- [x] 2.3 更新 `get_holdings` 等统计逻辑，确保正确处理（或忽略）`pending` 状态的交易。

## 3. 历史净值 Provider

- [x] 3.1 在 `src/provider/` 下新增 `lsjz` 接口的响应结构和 Provider 实现。
- [x] 3.2 在 `Aggregator` 中集成该接口支持，或者提供独立的 `fetch_by_date` 能力。


## 4. 结算引擎与命令集成

- [x] 4.1 修改 `Buy` 命令逻辑：当 `auto` 获取不到当日净值时，创建 `pending` 记录。
- [x] 4.2 在 `src/main.rs` 的 `sync_funds` 流程中增加“自动结算”环节。
- [x] 4.3 扫描并核销所有 `pending` 记录，自动补全缺失的份额。
- [x] 4.4 验证全流程：买入 (Pending) -> 同步 (Settled) -> 统计正确。
