## 1. 基础架构与依赖

- [x] 1.1 在 `skills/fund-manager/Cargo.toml` 中添加 `inquire` 依赖（用于 CLI 交互）。
- [x] 1.2 修改 `skills/fund-manager/src/db.rs` 中的 `init_db`，执行数据库迁移为 `fund` 表增加元数据列（类型、风险、经理、公司、费率、同步时间）。
- [x] 1.3 更新 `db.rs` 中的 `Fund` 结构体及相关数据库操作函数（如 `get_fund_by_code_or_name`），确保支持新字段。

## 2. 数据提供者 (Provider) 扩展

- [x] 2.1 在 `provider/mod.rs` 中扩展 `FundData` 结构体，添加元数据字段。
- [x] 2.2 实现 `DetailProvider` 以对接东方财富详情 API，并解析 `api.md` 中定义的 JSON 字段。
- [x] 2.3 实现 `SearchProvider` 以对接同花顺搜索建议 API，并使用正则解析 JSONP 响应。
- [x] 2.4 更新 `aggregator.rs`，支持在 `fetch_all` 流程中按需抓取并合并详情数据。

## 3. 智能解析逻辑 (Smart Resolver)

- [x] 3.1 实现 `resolve_fund` 核心函数：封装“本地查找 -> 远程搜索 -> 交互式选择”的完整链路。
- [x] 3.2 在 `resolve_fund` 中集成 `inquire::Select`，当搜索结果超过 1 条时弹出选择菜单。
- [x] 3.3 实现自动同步逻辑：在识别到代码后，检查本地记录的 `last_sync_at`，若缺失或超过 30 天则触发同步。

## 4. 业务逻辑集成与新功能

- [x] 4.1 修改 `main.rs` 中的 `Buy` 命令，将原本简单的基金查找替换为 `resolve_fund`。
- [x] 4.2 修改 `main.rs` 中的 `Sell` 命令，同步引入 `resolve_fund` 支持名称模糊卖出。
- [x] 4.3 在 `cli.rs` 和 `main.rs` 中新增 `fund sync [--force]` 命令，用于全量或强制更新本地元数据缓存。

## 5. 测试与验证

- [x] 5.1 为 `SearchProvider` 和 `DetailProvider` 编写单元测试，使用模拟响应验证解析正确性。
- [x] 5.2 编写集成测试验证“输入名称 -> 自动识别代码 -> 补全数据库详情”的完整业务流程。
## 6. 设计修正 (Fallback & Unified UA)

- [x] 6.1 在 `provider/mod.rs` 中增加一个统一的 `build_http_client` 函数，配置标准的 User-Agent。
- [x] 6.2 将所有 Provider (`eastmoney_js.rs`, `eastmoney_html.rs`, `eastmoney_details.rs`, `ths_search.rs`) 的 `reqwest::Client` 替换为统一构建方法。
- [x] 6.3 修改 `resolver.rs`，如果输入是 6 位纯数字，直接调用 `sync_fund_details` (即东方财富接口)，跳过同花顺搜索。
