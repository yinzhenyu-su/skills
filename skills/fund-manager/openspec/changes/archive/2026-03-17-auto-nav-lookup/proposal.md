## Why

当前交易创建时（如 `fund buy --date 2026-03-13`）只检查指定日期当天是否有 NAV，如果没有就直接标记为 pending。但实际上不同基金的净值公布时间不同（股票型当天出，QDII 可能 T+1 或 T+2），且节假日也会顺延。用户期望系统能自动往后查找最近的可用净值。

## What Changes

1. **新增智能 NAV 查找功能**：当指定日期的 NAV 不可用时，自动往后查找 20 天内最近的可用净值
2. **移除 `--auto` 参数**：默认就是自动模式，不需要用户显式传入
3. **用户提示**：当实际使用的 NAV 日期与指定日期不同时，提示用户

## Capabilities

### New Capabilities

- `smart-nav-lookup`: 智能 NAV 查找，支持往后查找 20 天内的可用净值，处理节假日顺延场景

### Modified Capabilities

- `asynchronous-settlement`: 现有的异步结算逻辑需要更新，以支持新的 NAV 查找策略

## Impact

- `src/db.rs`: 新增 `find_next_available_nav()` 函数
- `src/main.rs`: buy/sell/import 命令移除 `--auto` 参数，改用智能查找
- `src/cli.rs`: 更新 clap 定义
