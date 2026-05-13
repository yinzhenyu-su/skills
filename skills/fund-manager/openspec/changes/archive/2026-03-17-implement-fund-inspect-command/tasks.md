## 1. 数据模型与数据库准备

- [x] 1.1 在 `src/db.rs` 中新增 `fund_analysis` 表的创建逻辑。
- [x] 1.2 在 `src/db.rs` 中新增 `fund_analysis` 表的 CRUD 方法（存储和查询快照）。
- [x] 1.3 在 `src/provider/mod.rs` 中扩展 `FundData` 结构体，支持分析字段。

## 2. Morningstar Provider 实现

- [x] 2.1 创建 `src/provider/morningstar.rs` 并定义解析晨星 JSON 的 Rust 结构体。
- [x] 2.2 实现 `MorningstarProvider` 的 `fetch` 方法。
- [x] 2.3 在 `src/provider/mod.rs` 中注册新的 Provider。

## 3. CLI 与 报告展示

- [x] 3.1 在 `src/cli.rs` 中新增 `inspect <FUND>` 子命令。
- [x] 3.2 在 `src/main.rs` 中实现 `handle_inspect` 逻辑。
- [x] 3.3 实现 `format_inspect_report` 函数，使用 `comfy_table` 展示深度体检报告。
- [x] 3.4 实现 `investor_return_gap` 的计算逻辑（基金收益 - 投资者收益）。

## 4. 逻辑集成与测试

- [x] 4.1 在 `inspect` 逻辑中加入“本地缓存检查”，决定是否更新晨星数据。
- [x] 4.2 编写单元测试，验证晨星 JSON 的解析逻辑。
- [x] 4.3 编写集成测试，确保 `fund inspect` 命令能够正确输出。
