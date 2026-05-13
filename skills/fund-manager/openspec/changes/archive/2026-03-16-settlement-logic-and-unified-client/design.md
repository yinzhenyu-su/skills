## Context

目前 `fund-manager` 的数据拉取依赖于多个 `Provider`，每个 `Provider` 独立构建 `reqwest::Client` 并硬编码 User-Agent。基金买入操作在 `auto` 模式下强依赖于当日净值，若官方未公布，则会导致买入失败或数据错误。

## Goals / Non-Goals

**Goals:**
- 提供统一的、可配置的 HTTP 客户端。
- 允许记录缺失净值的“预买入”交易。
- 实现 `sync` 命令自动补全 `pending` 交易的份额。
- 引入支持历史日期查询的 `lsjz` 数据接口。

**Non-Goals:**
- 本阶段不实现 QDII 基金的实时估值（Real-time NAV Estimation）。
- 不对现有的卖出逻辑进行大规模重构（除非涉及结算状态）。

## Decisions

### 1. 统一 HTTP 客户端
- **选择**: 在 `src/provider/mod.rs` 中提供一个 `get_client()` 函数，从 `config` 中读取 UA。
- **理由**: 方案 A (Utility Function) 对现有代码侵入性最小，且能快速实现 Header (如 Referer) 的统一注入。

### 2. 数据库表迁移
- **选择**: 将 `transaction_log` 的 `shares` 和 `nav` 设为可空，并增加 `status` (TEXT) 字段。
- **理由**: 相比于使用 `0` 作为特殊值，`NULL` 配合 `status` 枚举能更清晰地表达业务语义，避免统计误差。

### 3. 结算逻辑触发
- **选择**: 将结算逻辑集成到 `fund sync` 命令中。
- **理由**: 用户已经习惯通过 `sync` 刷新数据，在刷新净值历史的同时自动对账是最自然的交互。

### 4. 历史数据查询接口
- **选择**: 使用东方财富 `lsjz` 接口。
- **理由**: 该接口支持通过 `startDate` 和 `endDate` 指定日期，是回填特定日期成交价的最佳选择。

## Risks / Trade-offs

- **[风险] 接口 Referer 校验** → **[缓解]** 在 `get_client()` 中默认添加 `https://fundf10.eastmoney.com/` 作为 Referer。
- **[风险] 结算失败（如节假日）** → **[缓解]** 结算引擎需具备幂等性，未获取到净值时不报错，保持 `pending` 状态待下次重试。
- **[风险] 数据库迁移兼容性** → **[缓解]** `init_db` 中需要增加对现有 `transaction_log` 表的 `ALTER TABLE` 逻辑。
