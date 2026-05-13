## Why

当前系统通过东财（Eastmoney）抓取基金元数据（如类型、风险等级、基金经理等）的逻辑不完整且解析不稳定，导致 `fund list` 等命令显示的基金信息大量缺失。晨星（Morningstar）提供更专业、结构化的 JSON 接口，能够显著提升数据的准确性和覆盖率。

## What Changes

- **新增 Morningstar Provider**：实现一个新的数据提供者，通过晨星 API 获取基金的静态元数据和费率信息。
- **混合同步策略**：在 `fund sync` 逻辑中，优先使用晨星获取元数据，同时保留东财作为净值数据（NAV）的主要来源，以兼顾数据的专业性和时效性。
- **聚合逻辑优化**：改进 `Aggregator`，确立字段合并的优先级，确保高质量的晨星元数据能够正确覆盖或补全东财数据。

## Capabilities

### New Capabilities
- `morningstar-metadata-sync`: 实现通过晨星 API 抓取基金类型、风险等级、基金经理、公司、成立日期及详细费率。

### Modified Capabilities
- `metadata-sync`: 调整元数据同步的来源优先级和合并策略。

## Impact

- `skills/fund-manager/src/provider/`: 新增 `morningstar.rs`。
- `skills/fund-manager/src/provider/aggregator.rs`: 集成 `MorningstarProvider` 并调整优先级。
- `skills/fund-manager/src/sync.rs`: 修改 `sync_funds` 逻辑，支持混合数据同步模式。
- 依赖项：可能需要添加 `serde_json`（如果尚未添加）用于处理晨星的 JSON 响应。
