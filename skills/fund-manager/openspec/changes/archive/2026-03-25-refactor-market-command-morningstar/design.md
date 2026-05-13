## Context

目前 `fund market` 的实现位于 `morningstar_market.rs` 和 `main.rs`。当前的实现仅解析了 `chinaEquity` 和 `globalEquity` 数组中的基础字段（名称、价格、涨跌、涨跌幅）。现有的 `IndexData` 结构体过于简单，无法支持 52 周区间、市场状态和日内趋势等高级功能。

## Goals / Non-Goals

**Goals:**
- 重构 `morningstar_market.rs` 中的数据模型以匹配完整 API。
- 提取并解析 `exchangeRate`, `commodity`, `hotAssets` 等新分类。
- 在 CLI 中实现动态列展示（根据参数显示 52 周区间或趋势图）。
- 实现通用的 `MarketItem` 归一化逻辑。

**Non-Goals:**
- 不会实现跨天或历史 K 线图（仅限于 API 提供的 `ts` 日内点位）。
- 不会修改其他 Provider（如东财）的逻辑。

## Decisions

### 1. 数据模型归一化 (Data Model Normalization)
- **决策**: 引入通用的 `MarketItem` 结构体和 `MarketCategory` 枚举。
- **理由**: Morningstar API 将不同类型的资产放在不同的 JSON 数组中，但它们的大部分字段（price, chg, pct, w52h, status）是通用的。归一化可以简化 UI 渲染逻辑。
- **备选方案**: 为每个分类创建不同的 Struct。但这会导致 `main.rs` 中出现大量重复的渲染代码。

### 2. 趋势图渲染 (Sparkline Rendering)
- **决策**: 实现一个简单的 `render_sparkline(data: &[f64]) -> String` 函数，使用 Unicode 块元素 (`  ▂▃▄▅▆▇█`)。
- **理由**: 这种方式在命令行中非常轻量且美观，不需要额外的图形库依赖。
- **逻辑**: 将输入数据映射到 0-7 的高度范围。

### 3. 52 周水位条 (Range Bar)
- **决策**: 使用百分比计算 `(current - low) / (high - low)` 并通过 `■` 和 `□` 绘制进度条。
- **理由**: 相比于单纯的百分比数字，进度条能更直观地展示当前价格在年度范围内的相对位置。

### 4. 接口解析逻辑
- **决策**: 使用 `serde(flatten)` 或通用的 `Map<String, Value>` 处理 `WatchListResponse` 中的所有 Key。
- **理由**: 接口返回的 Key 具有一定的动态性（如 `ceGroupTime`, `coGroupTime` 等时间戳 Key），我们需要灵活捕获核心数据数组。

## Risks / Trade-offs

- **[Risk]** Morningstar API 响应过大 (JSON 包含大量 `ts` 数据) → **Mitigation**: Rust 的 `serde` 解析速度极快，且单次请求避免了多次网络往返。如果解析成为瓶颈，可以考虑 `Deserialize` 时跳过不必要的字段。
- **[Trade-off]** 终端宽度限制 → **Mitigation**: 默认不显示详细信息和趋势图，仅在用户显式通过 `-d` 或 `-t` 指定时才增加对应的列。
- **[Risk]** 部分资产缺少 `ts` 或 `w52h` 数据 → **Mitigation**: UI 渲染层需要处理 `Option`，对于缺失数据显式显示 `N/A` 或留空，保持表格整齐。
