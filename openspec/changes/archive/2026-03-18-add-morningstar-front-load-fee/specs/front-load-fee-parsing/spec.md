## ADDED Requirements

### Requirement: 前端申购费率解析
系统 SHALL 从晨星 `fees` 接口的 `frontLoadFee` 字段提取前端申购费率。

#### Scenario: 解析 frontLoadFee 数组
- **WHEN** 晨星 `fees` 接口返回 `frontLoadFee: [{"floor": 0.0, "fee": 1.5, "feeUnit": 2.0, ...}]`
- **THEN** 系统 SHALL 解析为 `fee_rate: "1.5%"`（第一档，0-50万适用）

#### Scenario: 处理固定金额费率
- **WHEN** 晨星 `fees` 接口返回 `frontLoadFee: [{"floor": 5000000.0, "fee": 1000.0, "feeUnit": 1.0, ...}]`
- **THEN** 系统 SHALL 识别 `feeUnit: 1.0` 表示固定金额

#### Scenario: 处理无 frontLoadFee 的基金
- **WHEN** 晨星 `fees` 接口返回 `frontLoadFee: []` 或字段不存在
- **THEN** 系统 SHALL 将 `fee_rate` 设置为 `None`
