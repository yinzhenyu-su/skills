## MODIFIED Requirements

### Requirement: 费率数据提取
系统 SHALL 从晨星的 `fees` 接口提取管理费、托管费、销售服务费**以及前端申购费率**。

#### Scenario: 提取费率
- **WHEN** 晨星 `fees` 接口返回 `managementFee: 1.2%`
- **THEN** 系统 SHALL 将其格式化为字符串 "1.2%" 并存入 `management_fee` 字段。

#### Scenario: 提取前端申购费率
- **WHEN** 晨星 `fees` 接口返回 `frontLoadFee: [{"floor": 0.0, "fee": 1.5, "feeUnit": 2.0, ...}]`
- **THEN** 系统 SHALL 将第一档费率解析为 `fee_rate` 字段（格式："1.5%"）
