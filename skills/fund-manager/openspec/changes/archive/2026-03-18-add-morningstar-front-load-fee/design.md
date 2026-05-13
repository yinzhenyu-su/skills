## Context

当前晨星 Provider (`morningstar.rs`) 的 `FeesData` 结构体仅包含：
- `management_fee`: 管理费
- `custodian_fee`: 托管费
- `distribution_fee`: 销售服务费

晨星 fees API 实际返回的数据结构（以 457001 为例）：

```json
{
  "managementFee": 1.2,
  "custodianFee": 0.2,
  "distributionFee": null,
  "frontLoadFee": [
    {"floor": 0.0, "fee": 1.5, "feeUnit": 2.0, "floorUnit": 1.0},
    {"floor": 500000.0, "fee": 1.2, "feeUnit": 2.0, "floorUnit": 1.0},
    {"floor": 1000000.0, "fee": 0.6, "feeUnit": 2.0, "floorUnit": 1.0},
    {"floor": 5000000.0, "fee": 1000.0, "feeUnit": 1.0, "floorUnit": 1.0}
  ]
}
```

其中 `frontLoadFee` 是费率阶梯数组，`feeUnit: 2.0` 表示百分比（%），`feeUnit: 1.0` 表示固定金额（元）。

## Goals / Non-Goals

**Goals:**
- 解析 `frontLoadFee` 结构并提取基准申购费率（0-50万档，即第一档）
- 将解析结果映射到 `FundData.fee_rate` 字段
- 新增基于真实 457001 数据的业务测试用例

**Non-Goals:**
- 不实现完整费率阶梯计算（只需获取基准费率用于展示）
- 不修改晨星 API 的调用方式
- 不改变现有 aggregator 的合并逻辑

## Decisions

### Decision 1: 费率数据结构设计

**Choice:** 在 `FeesData` 中添加 `front_load_fee: Option<Vec<FrontLoadFeeTier>>`

**Rationale:** 晨星返回的 `frontLoadFee` 是一个数组，代表不同申购金额区间的费率。保持原始结构便于后续扩展。

```rust
struct FrontLoadFeeTier {
    floor: f64,        // 金额下限
    fee: f64,          // 费率值
    fee_unit: f64,    // 1.0=固定金额, 2.0=百分比
    floor_unit: f64,  // 1.0=元
}
```

### Decision 2: 基准费率提取策略

**Choice:** 使用第一档（floor = 0.0）的费率作为 `fee_rate`

**Rationale:** 第一档（0-50万）适用于大多数个人投资者的小额定投场景，是用户最关心的基准费率。

**Alternative considered:** 动态计算 - 根据用户输入金额匹配对应档位
- **Rejected:** 当前 `FundData.fee_rate` 是单一字段，无法存储多档费率。简单起见，先使用第一档。

### Decision 3: feeUnit 解析

**Choice:** `feeUnit = 2.0` 时按百分比处理（`fee / 100`），`feeUnit = 1.0` 时按固定金额处理

**Rationale:** 晨星文档定义如此，需正确区分百分比和固定金额。

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| 某些基金可能没有 `frontLoadFee`（如货币基金） | 使用 `Option`，无数据时保持 `None` |
| 测试数据依赖外部 API，可能不稳定 | 测试用例使用 `SKIP_SYNC=1` 避免真实 API 调用 |
| 费率精度问题 | 使用 `rust_decimal::Decimal` 避免浮点误差 |

## Open Questions

1. **Q:** 是否需要将固定金额档位（feeUnit=1.0）转换为等效百分比？
   **A:** 暂不需要，当前只需存储基准费率字符串供展示。

2. **Q:** 晨星 API 返回的费率是原价还是优惠价？
   **A:** 晨星通常显示标准费率，实际优惠需以购买平台为准。当前获取的 `frontLoadFee` 是标准费率。
