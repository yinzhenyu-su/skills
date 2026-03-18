## Context

当前系统在同步基金净值时，主要依赖天天基金的历史净值 (`lsjz`) 和实时估值 (`fundgz`) 接口。经过实测发现，当带上日期范围参数时，天天基金 API 对 `pageSize` 有严格限制（通常为 20 或 50），而代码中硬编码的 `1000` 会导致服务端直接返回空数据。此外，`Aggregator` 的错误忽略机制掩盖了这些请求失败，使得问题极难被察觉。

## Goals / Non-Goals

**Goals:**
- 修复 `EastmoneyLsjzProvider` 的分页请求限制，确保历史净值能正确拉取。
- 增强 `EastmoneyJsProvider` 的字符编码解析，提升抓取成功率。
- 在 `Aggregator` 中引入错误上报机制，不再静默忽略所有异常。
- 统一 `Referer` 字段的规范化管理。

**Non-Goals:**
- 不涉及数据库 Schema 的修改。
- 不涉及交易结算逻辑的变更（除非是由净值缺失引起的结算失败）。
- 不引入新的第三方库。

## Decisions

### 1. 降低 `pageSize` 并引入分页循环
- **决策**：将 `EastmoneyLsjzProvider::fetch_range` 中的 `pageSize` 设为 `20`，并根据 `TotalCount` 执行分页抓取循环。
- **理由**：20 是经过实测最安全、最稳定的单页大小。
- **替代方案**：改为 50 或 100。实测显示 100 在某些环境下仍可能被拦截，20 最为稳妥。

### 2. `Aggregator` 错误可见性提升
- **决策**：修改 `Aggregator::fetch_at_date`，当所有 Provider 都失败时，返回一个聚合后的详细错误信息。如果是部分 Provider 失败，则打印警告日志。
- **理由**：完全静默会导致调试极其困难；显式报错能提醒用户检查网络或接口状态。

### 3. 基于 `encoding_rs` 的混合编码处理 (如果需要)
- **决策**：在解析 `EastmoneyJsProvider` 返回的文本时，优先检查 `Content-Type`。如果存在 `GBK` 声明，则使用合适的解码器。
- **理由**：天天基金的部分接口仍在使用旧式的 GBK 或混合编码。

## Risks / Trade-offs

- **[Risk]** 分页抓取会增加 HTTP 请求次数 → **[Mitigation]** 仅在同步较长历史区间时发生，且通过 `Tokio` 并发处理单页请求来降低总耗时。
- **[Risk]** `Aggregator` 显式报错可能导致同步流程中断 → **[Mitigation]** 仅在“所有”数据源均失效且无法获取到关键数据时才返回错误。
