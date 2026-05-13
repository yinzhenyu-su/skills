## Why

当前 fund-manager 在基金列表和持仓视图中展示的“最新净值”来自本地已确认净值记录，因此通常是 T-1，遇到节假日、QDII 或披露延迟时还可能退化为 T-N。用户在交易时段需要看到更接近盘中真实波动的估算结果，以便快速了解当前持仓变化。

## What Changes

- 新增基于实时市场指数涨跌幅推算基金盘中估值的能力，用于持仓展示场景。
- 为支持估值映射，补充基金元数据中的业绩比较基准信息，并结合基金名称规则与手工覆盖规则解析目标指数。
- 调整基金持仓展示逻辑，同时展示确认净值与实时估算值，并在无法可靠估算时给出明确降级说明。
- 对不同基金类型采用差异化策略：宽基指数基金优先精确估算，QDII/黄金使用代理指数，货币/债券/无法映射的基金不进行实时估算。

## Capabilities

### New Capabilities

- `realtime-fund-valuation`: 根据基金基准指数或代理指数的实时涨跌幅计算盘中估算净值与估算市值，并为不同基金类型提供降级和提示规则。

### Modified Capabilities

- `fund-valuation-display`: 基金列表和持仓展示从仅显示确认净值，扩展为区分确认净值与实时估算值，并在无法估算或休市时给出明确状态。

## Impact

- Affected code: `skills/fund-manager/src/provider/mod.rs`, `skills/fund-manager/src/provider/morningstar.rs`, `skills/fund-manager/src/provider/morningstar_market.rs`, `skills/fund-manager/src/main.rs`
- New logic: 基准指数解析、估值映射规则、盘中估值计算与降级展示
- External systems: 继续依赖 Morningstar 基金与市场接口，不新增外部依赖
