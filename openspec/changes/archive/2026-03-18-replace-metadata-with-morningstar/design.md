## Context

目前，基金元数据的同步依赖于从东财 HTML 页面抓取。这种方式解析复杂且数据不全（如风险等级、基金经理等经常缺失）。晨星（Morningstar）提供了一套基于 REST API 的 JSON 接口，数据格式稳定且专业。

## Goals / Non-Goals

**Goals:**
- 实现 `MorningstarProvider`，调用 `/common-data` 和 `/fees` 接口。
- 确立混合数据源模式：元数据来自晨星，净值数据来自东财。
- 提升 `fund list` 和 `fund status` 中基金信息的完整度。

**Non-Goals:**
- 不完全废弃东财接口，因为东财的净值更新时效性更好。
- 不引入重度的网页爬虫框架（如 Selenium），保持轻量级请求。

## Decisions

### 1. 并行请求与聚合
晨星的数据分布在不同的 endpoint 中。
- **方案**：在 `MorningstarProvider` 中使用 `tokio::join!` 或 `futures::future::join_all` 并行请求 `/common-data` 和 `/fees`。
- **理由**：减少总响应时间，且这两个请求是互不依赖的。

### 2. 聚合优先级调整
目前的 `Aggregator` 简单地以后续 Provider 覆盖前面的。
- **调整**：显式调整 Provider 的添加顺序：
    1. `MorningstarProvider`（提供核心元数据）
    2. `EastmoneyJsProvider`（提供最新净值和名称）
    3. `EastmoneyHtmlProvider`（兜底其他信息）
- **理由**：确保晨星的高质量元数据被优先采纳，同时东财的实时净值能覆盖晨星可能延迟的净值。

### 3. 数据映射表
晨星的字段名与我们的 `FundData` 结构体需要精确映射。
- `morningstarCategory` -> `fund_type`
- `riskLevel` -> `risk_level`
- `managerName` -> `manager`
- `managementFee` -> `management_fee`

## Risks / Trade-offs

- **[风险] 晨星接口频率限制** → **[缓解]** 仅在元数据缺失或过期（30天）时请求晨星，避免高频调用。
- **[风险] 基金代码匹配失败** → **[缓解]** 晨星接口支持直接在 URL 中使用 6 位代码，具有较高的通用性；若晨星失败，则回退到原有的东财流程。
- **[风险] 净值时效性差异** → **[缓解]** 维持以东财净值为准的策略。
