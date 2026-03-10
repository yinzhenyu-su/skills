## ADDED Requirements

### Requirement: Aggregate Fund Data
系统 SHALL 协调 JS 提供商和 HTML 提供商，将获取到的信息合并。

#### Scenario: Merging data sources
- **WHEN** JS 提供商提供名称和净值，HTML 提供商提供费率
- **THEN** 系统生成完整的 `Fund` 结构，包含代码、名称、最新净值和费率。

### Requirement: Timeout and Error Recovery
系统 SHALL 对所有网络请求设置 10 秒超时，并在某一个提供商失败时尽量返回已有数据。

#### Scenario: Partial data on failure
- **WHEN** HTML 费率解析失败，但 JS 净值请求成功
- **THEN** 系统仍应返回包含最新净值的基金信息，费率设为默认值或 None。
