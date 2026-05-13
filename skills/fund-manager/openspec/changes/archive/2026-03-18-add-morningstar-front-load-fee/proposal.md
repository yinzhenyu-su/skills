## Why

当前晨星 Provider 仅解析 `managementFee`、`custodianFee` 和 `distributionFee`，但晨星 fees API 实际上还返回 `frontLoadFee`（前端申购费率）数据，这是基金买入时收取的一次性费用。缺少这部分数据导致用户无法自动获取申购费率。

同时，现有的 457001 盈亏计算测试使用手动插入的静态数据，无法验证与真实 API 交互时的业务逻辑正确性。

## What Changes

1. **增强晨星 Fees 解析**：在 `FeesData` 结构体中添加 `frontLoadFee` 字段及其解析逻辑
2. **新增申购费率获取能力**：从 `frontLoadFee` tier 结构中提取适用于小额定投的基准费率（0-50万档）
3. **新增基于真实数据的业务测试用例**：使用 457001 基金的真实净值和费率数据，验证买入、持有、卖出全流程的盈亏计算正确性

## Capabilities

### New Capabilities

- `front-load-fee-parsing`: 解析晨星 `frontLoadFee` 数据结构，提取申购费率用于买入计算

### Modified Capabilities

- `morningstar-metadata-sync` / `费率数据提取`: 现有"费率数据提取"需求未完整实现，当前只提取管理费/托管费/销售费，**缺少前端申购费率**。本次变更将补充 `frontLoadFee` 的解析。

## Impact

- **代码变更**：`skills/fund-manager/src/provider/morningstar.rs`
- **测试变更**：`skills/fund-manager/tests/cli_tests.rs`
- **数据结构**：`FeesData` 结构体需添加新字段
