## Context

当前系统在创建交易时（如 `fund buy --money 1000 --date 2026-03-13`），仅检查指定日期当天是否有 NAV：
- 有 NAV → 直接 settled
- 没有 → 直接 pending

这种方式无法处理：
1. 节假日顺延（指定周五但周五 NAV 还没出）
2. QDII 基金延迟公布（T+1、T+2）
3. 用户期望系统自动往后找可用净值

## Goals / Non-Goals

**Goals:**
- 支持指定日期后往后查找 20 天内的可用 NAV
- 移除 `--auto` 参数，默认自动
- 当实际使用日期与指定日期不同时，提示用户

**Non-Goals:**
- 不修改现有 pending 交易的结算逻辑（已在 `fund sync` 中处理）
- 不添加 QDII 基金类型自动检测功能

## Decisions

1. **查找范围**: 固定 20 天
   - 理由：足够覆盖节假日 + QDII 延迟，且不会导致过长的等待时间

2. **存储策略**: 记录实际使用的 NAV 日期
   - 理由：便于后续 sync 时正确匹配已结算记录

3. **CLI 变化**: 移除 `--auto`，buy/sell/import 默认使用智能查找
   - 理由：简化用户体验，auto 模式应该是默认而非可选

## Risks / Trade-offs

- [风险] 查找 20 天后仍未找到 NAV
  -  mitigation: 仍然创建 pending 交易，后续 sync 会继续尝试

- [风险] NAV 日期顺延导致份额计算不直观
  -  mitigation: 输出明确提示，告诉用户使用了哪天的 NAV
