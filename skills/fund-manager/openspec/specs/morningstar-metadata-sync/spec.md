## ADDED Requirements

### Requirement: 晨星数据源集成
系统 SHALL 集成晨星（Morningstar）API 作为基金元数据的核心来源。

#### Scenario: 从晨星抓取元数据
- **WHEN** 系统对基金 "163813" 执行元数据同步
- **THEN** 系统 SHALL 并发或顺序请求 `common-data` 和 `fees` 接口
- **AND** 系统 SHALL 解析出 `morningstarCategory`（类型）、`riskLevel`（风险等级）、`managerName`（基金经理）、`companyName`（基金公司）、`inceptionDate`（成立日期）以及各项费率。

### Requirement: 晨星数据解析规范
系统 SHALL 能够准确处理晨星 JSON 响应中的标准化字段。

#### Scenario: 准确解析风险等级
- **WHEN** 晨星返回 `riskLevel: "中风险(R3)"`
- **THEN** 系统 SHALL 将其完整存入数据库的 `risk_level` 字段。

### Requirement: 费率数据提取
系统 SHALL 从晨星的 `fees` 接口提取管理费、托管费、销售服务费以及前端申购费率。

#### Scenario: 提取费率
- **WHEN** 晨星 `fees` 接口返回 `managementFee: 1.2%`
- **THEN** 系统 SHALL 将其格式化为字符串 "1.2%" 并存入 `management_fee` 字段。

#### Scenario: 提取前端申购费率
- **WHEN** 晨星 `fees` 接口返回 `frontLoadFee: [{"floor": 0.0, "fee": 1.5, "feeUnit": 2.0, ...}]`
- **THEN** 系统 SHALL 将第一档费率解析为 `fee_rate` 字段（格式："1.5%"）
