## Why

当前系统提供的 `fund market`（原 `index` 指令）功能过于基础，仅能展示 A 股及少量海外指数的点位和涨跌。
晨星（Morningstar）的 `watch-list` 接口实际上提供了包含 A 股、港股、全球指数、外汇、大宗商品在内的全维度市场数据，
并附带了 52 周高低位、实时市场状态以及部分资产的日内分时趋势（`ts`）。
重构该指令将使其从简单的“点位查询器”升级为专业的“市场实时看板”，提升工具的实用价值和专业性。

## What Changes

- **扩展数据源范围**：将 `fund market` 的覆盖范围从单一指数扩展到 **外汇 (Forex)** 和 **大宗商品 (Commodity)**。
- **深度数据展示**：引入 **52 周高低位百分比** 展示，帮助用户直观判断当前估值水位。
- **实时状态感知**：增加 **市场开盘/休市状态** (🟢/🔴) 显示，提供更清晰的实时背景。
- **日内趋势可视化**：针对黄金、汇率及欧洲指数等提供分时数据（`ts`）的资产，在终端绘制 **ASCII Sparkline (趋势图)**。
- **灵活的指令参数**：
    - 支持按分类过滤（如 `--fx`, `--com`, `--index`）。
    - 支持详细视图 (`--detail` / `-d`) 查看水位。
    - 支持趋势视图 (`--trend` / `-t`) 查看日内动量。
- **配置驱动**：允许用户在 `config.yaml` 中自定义默认关注列表。

## Capabilities

### New Capabilities
- `market-dashboard`: 提供多层级、可插拔的市场看板系统，支持 52 周区间、日内趋势图及多资产类别过滤显示。

### Modified Capabilities
- `realtime-index-query`: 扩展原有的实时指数查询逻辑，从仅支持 Equity 扩展到支持全类别 Market 资产及其深度元数据（w52h, status, ts 等）。

## Impact

- `skills/fund-manager/src/provider/morningstar_market.rs`: 数据模型重构，增强解析逻辑。
- `skills/fund-manager/src/main.rs`: 指令逻辑重构，实现多视图渲染。
- `skills/fund-manager/src/config.rs`: 增加默认行情关注列表配置支持。
