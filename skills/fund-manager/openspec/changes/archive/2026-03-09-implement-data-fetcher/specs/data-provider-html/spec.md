## ADDED Requirements

### Requirement: HTML Detail Page Request
系统 SHALL 向 `https://fund.eastmoney.com/{code}.html` 发起异步 HTTP GET 请求。

#### Scenario: Successful HTML request
- **WHEN** 给定基金代码 "160119"
- **THEN** 系统下载该页面的 HTML 内容。

### Requirement: Fee Rate Extraction
系统 SHALL 解析 HTML 中的 CSS 选择器 `.nowPrice` 对应的 `span` 元素，并提取其文本内容（如 "0.12%"）。

#### Scenario: Parsing fee rate
- **WHEN** HTML 中存在 `<span class="nowPrice">0.12%</span>`
- **THEN** 系统解析出费率为 0.0012 (即 0.12%)。
