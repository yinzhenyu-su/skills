## ADDED Requirements

### Requirement: 智能 NAV 查找
当创建交易时，系统必须自动往后查找 20 天内最近的可用净值。

#### Scenario: 指定日期有 NAV
- **WHEN** 用户执行 `fund buy 000300 --money 1000 --date 2026-03-13`，且 2026-03-13 有 NAV
- **THEN** 系统使用 2026-03-13 的 NAV 直接结算

#### Scenario: 指定日期无 NAV，顺延后有
- **WHEN** 用户执行 `fund buy 000300 --money 1000 --date 2026-03-13`（周五），且 2026-03-13 无 NAV，但 2026-03-16 有 NAV
- **THEN** 系统使用 2026-03-16 的 NAV 结算，并在输出中提示 "Note: NAV for 2026-03-13 not available, used 2026-03-16"

#### Scenario: 20 天内都无 NAV
- **WHEN** 用户执行 `fund buy 000300 --money 1000 --date 2026-03-13`，且 20 天内都无 NAV
- **THEN** 系统创建 pending 交易，记录原始日期 2026-03-13

#### Scenario: 未指定日期，默认今天
- **WHEN** 用户执行 `fund buy 000300 --money 1000`（未指定 --date）
- **THEN** 系统使用今天的日期，并往后查找 20 天

### Requirement: 默认自动模式
buy、sell、import 命令默认使用自动 NAV 查找，无需 --auto 参数。

#### Scenario: 无 --auto 参数自动结算
- **WHEN** 用户执行 `fund buy 000300 --money 1000`
- **THEN** 系统自动尝试查找 NAV，能找到则直接结算

#### Scenario: 移除 --auto 后的行为
- **WHEN** 用户执行 `fund buy 000300 --money 1000 --auto`
- **THEN** 系统提示 "Unknown option: --auto" 并退出

### Requirement: 网络获取
当本地数据库找不到指定日期的 NAV 时，系统必须尝试从网络 API 获取。

#### Scenario: 本地无 NAV，API 有
- **WHEN** 用户执行 `fund buy 000300 --money 1000 --date 2026-03-13`，本地数据库无 NAV，但 API 返回有效 NAV
- **THEN** 系统使用 API 返回的 NAV 结算，并将 NAV 保存到本地数据库

#### Scenario: 本地无 NAV，API 也无
- **WHEN** 用户执行 `fund buy 000300 --money 1000 --date 2026-03-13`，本地数据库和 API 都无 NAV
- **THEN** 系统往后查找 20 天，找到则使用，找不到则创建 pending 交易
