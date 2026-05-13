## ADDED Requirements

### Requirement: 鲁棒的分页抓取机制
系统在拉取历史净值时，必须能够自动识别并处理天天基金接口的分页限制。

#### Scenario: 历史净值区间超过单页上限
- **WHEN** 用户同步 000513 基金从 2026-03-01 到 2026-03-18 的历史净值
- **THEN** 系统应使用 pageSize=20 进行多次请求，直到抓取到所有 18 天的数据或 TotalCount 为止

### Requirement: 显式的 Aggregator 错误反馈
当所有数据源都无法提供有效数据时，Aggregator 必须抛出包含错误原因的 Result。

#### Scenario: 网络断开或所有 Provider 挂掉
- **WHEN** 用户执行同步命令且所有请求均由于超时或 403 失败
- **THEN** 系统应在终端显示“无法从任何数据源拉取有效数据”，并列出主要错误原因（如“超时”）

### Requirement: UTF-8 与 GBK 编码自动识别
`EastmoneyJsProvider` 必须能正确解析包含 `charset=gbk` 或 `UTF-8,gbk` 的 HTTP 响应。

#### Scenario: 解析带 GBK 声明的 JS 文件
- **WHEN** `EastmoneyJsProvider` 接收到包含非 UTF-8 字符但声明了 charset 的响应
- **THEN** 系统应能无乱码解析其中的基金名称和净值数据
