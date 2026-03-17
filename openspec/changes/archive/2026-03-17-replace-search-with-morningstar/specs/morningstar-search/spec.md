## ADDED Requirements

### Requirement: 晨星基金缓存搜索
系统 SHALL 能够调用晨星官方 `fund-cache` API 进行基金联想搜索。

#### Scenario: 成功搜索到多只基金
- **WHEN** 用户输入 "南方中证" 作为关键字进行搜索
- **THEN** 系统 SHALL 发起对 `https://www.morningstar.cn/cn-api/public/v1/fund-cache/南方中证` 的 GET 请求，并正确解析返回的 JSON 数组，提取 `symbol`、`fundNameArr` 和 `fundType` 字段

### Requirement: 搜索结果的展示
当远程搜索返回结果时，系统 SHALL 在展示列表或交互选择中包含基金的类型信息（如果可用）。

#### Scenario: 交互选择中显示基金类型
- **WHEN** 搜索返回多条结果且处于交互模式
- **THEN** 交互列表 SHALL 以 `[代码] 名称 (类型)` 的格式展示，例如 `[160119] 南方中证500ETF联接（LOF）A (股票型)`
