# 基金买卖所需字段来源确认表单

本文档用于确认 `fund-manager` 在“买入/卖出判断”场景下各字段的数据来源、可用性、风险和降级策略。

## 1. 字段来源确认表

| 业务字段 | 用途 | 主来源接口 | JSON 路径 | 状态 | 说明/风险 |
|---|---|---|---|---|---|
| `subscription_status` | 判断是否可申购 | Eastmoney `lsjz` | `Data.LSJZList[].SGZT` | 已验证 | 可从最新净值记录提取，如“开放申购/暂停申购” |
| `redemption_status` | 判断是否可赎回 | Eastmoney `lsjz` | `Data.LSJZList[].SHZT` | 已验证 | 可从最新净值记录提取，如“开放赎回/暂停赎回” |
| `redemption_fee_tiers` | 卖出费率分档 | Morningstar `fees` | `data.redemptionFee[]` | 已验证 | 需解析 `floor/fee/feeUnit/floorUnit` |
| `front_load_fee_tiers` | 买入费率分档 | Morningstar `fees` | `data.frontLoadFee[]` | 已验证 | 可能包含百分比或固定金额档 |
| `defer_load_fee_tiers` | 后端收费分档 | Morningstar `fees` | `data.deferLoadFee[]` | 已验证 | 可选能力，是否用于决策待产品确认 |
| `min_subscription_amount` | 最低买入额 | Morningstar `fees` | `data.minInvestment` | 已验证（口径待定） | 与 `shareClassFees[].minInvestment` 可能冲突，需定义基金级取值规则 |
| `limit_per_transaction` | 单笔限购金额 | Morningstar `fees` | `data.purchaseAndRedeem.applyingMaxIII/IV/VII/VIII` | 已验证（取最大值策略） | 取 `applyingMaxIII/IV/VII/VIII` 中的最大正值作为单笔限购额；若全为空则置 `None` |
| `management_fee` | 管理费参考 | Morningstar `fees` | `data.managementFee` | 已验证 | 单位通常为百分比数值（例如 `1.2`） |
| `custodian_fee` | 托管费参考 | Morningstar `fees` | `data.custodianFee` | 已验证 | 同上 |
| `distribution_fee` | 销售服务费参考 | Morningstar `fees` | `data.distributionFee` | 已验证 | 同上，可能为 `null` |
| `settlement_days` | 资金到账天数（T+N） | 暂无稳定来源 | 暂无 | 未验证 | 当前已确认接口中无直接 T+N 字段，需展示降级为“未知” |

## 2. 单位与分档解析规则（建议）

| 字段 | 含义 | 处理规则 |
|---|---|---|
| `feeUnit = 2.0` | 百分比 | `fee` 解释为百分比，入库时转换为小数（例如 `1.5` -> `0.015`） |
| `feeUnit = 1.0` | 固定金额 | `fee` 解释为固定金额（元） |
| `floor` | 分档起点 | 与 `floorUnit` 联合解释（金额/时间） |
| `floorUnit = 1.0` | 金额阈值 | 通常用于申购费分档 |
| `floorUnit = 10.0` | 天数阈值 | 常见于赎回费分档 |
| `floorUnit = 4.0` | 月份阈值 | 常见于后端收费分档 |

## 3. 降级策略（必须）

1. 当 `SGZT/SHZT` 缺失时，状态显示为 `未知`，不阻断 `status/inspect`。
2. 当 `redemptionFee` 缺失时，`sell/preview sell` 必须提示“费率数据不可用”，允许用户 `--fee` 手动覆盖。
3. 当 `minInvestment` 与 `shareClassFees[].minInvestment` 冲突时，按预设口径选取并在日志中记录来源。
4. 当 `purchaseAndRedeem` 字段语义未命中时，`limit_per_transaction` 置空并提示“限购信息未确认”。
5. `settlement_days` 无来源时统一展示 `-`，文案为“到账周期未知，以基金公司公告为准”。

## 4. 当前结论

- 已有稳定来源：`subscription_status`、`redemption_status`、`redemption_fee_tiers`、`front_load_fee_tiers`。
- 需规则落地后可用：`min_subscription_amount`、`limit_per_transaction`。
- 当前无稳定来源：`settlement_days`。
