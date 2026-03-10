## Context

该模块负责从天天基金网获取实时数据。由于没有官方公开的免费 API，我们采用“JS 接口 + HTML 解析”的混合方案来获取完整的基金信息。

## Goals / Non-Goals

**Goals:**
- 实现对 `fundgz.1234567.com.cn` JS 接口的请求和正则解析。
- 实现对基金详情页 HTML 的请求和 `.nowPrice` 费率的解析。
- 保证抓取过程的异步非阻塞，并支持超时处理。
- 抽象出 `Provider` Trait，方便未来扩展其他数据源。

**Non-Goals:**
- 不支持复杂的验证码绕过。
- 不保证 100% 的抓取成功率（受网络和反爬限制）。

## Decisions

### 1. 技术栈选择：`reqwest` + `scraper` + `regex`
- **Rationale**: `reqwest` 是 Rust 生态中最成熟的 HTTP 客户端。`scraper` 基于 CSS 选择器，解析 HTML 非常直观。`regex` 用于处理 JS 响应中非标准 JSON 的部分。

### 2. Provider 模式设计
- **Rationale**: 定义一个 `Provider` Trait，包含 `fetch_latest_nav` 和 `fetch_fee_rate` 等方法。这允许我们针对不同基金类型或来源实现不同的解析器。

### 3. 正则解析策略
- **Rationale**: JS 响应格式为 `jsonpgz({...});`。使用正则 `r"jsonpgz\((.*)\);"` 提取中间的 JSON 字符串，然后使用 `serde_json` 进行反序列化。

### 4. 费率解析策略
- **Rationale**: 访问 `https://fund.eastmoney.com/{code}.html`，定位 `span.nowPrice`。如果该元素不存在或解析失败，则返回 `None` 或默认值。

## Risks / Trade-offs

- **[Risk] 天天基金类名变更** → **Mitigation**: 在代码中集中管理选择器常量，并编写集成测试定期检查抓取是否失效。
- **[Risk] 网络超时导致 CLI 卡顿** → **Mitigation**: 为所有网络请求设置 5-10 秒的严格超时。
- **[Risk] IP 被封禁** → **Mitigation**: 默认不使用代理，但在请求头中添加常见的 `User-Agent` 以降低被识别为爬虫的风险。
