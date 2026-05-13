## Why

为了让基金管理工具能够自动化获取实时的基金数据（净值、名称、费率），我们需要实现一个健壮的数据抓取模块。当前用户手动录入数据效率低下且易出错，自动抓取能确保盈亏分析的实时性和准确性。

## What Changes

- **实现双源数据抓取策略**：
  - 使用 JS 接口 (`jsonpgz`) 快速获取最新的单位净值、基金名称和日期。
  - 使用 HTML 解析方案（访问基金详情页）获取折后申购费率（解析 CSS 类名为 `.nowPrice` 的 `span` 元素）。
- **统一数据模型转换**：将不同来源的数据（JS 响应、HTML DOM）统一转换为项目内部定义的 `Fund` 和 `NavHistory` 实体。
- **引入新的依赖**：集成 `scraper` 用于 HTML 解析，`regex` 用于 JS 响应处理。

## Capabilities

### New Capabilities
- `data-provider-js`: 实现对天天基金 JS 接口的请求和解析逻辑。
- `data-provider-html`: 实现对天天基金 HTML 页面的请求和特定 CSS 选择器（如 `.nowPrice`）的解析逻辑。
- `data-aggregator`: 协调多个数据源，合并并规范化输出完整的基金信息。

### Modified Capabilities
- 无

## Impact

- **网络模块**：新增针对不同 URL 模式的请求逻辑。
- **解析层**：新增正则提取器和 CSS 选择器提取器。
- **依赖**：新增 `scraper`, `regex`。
