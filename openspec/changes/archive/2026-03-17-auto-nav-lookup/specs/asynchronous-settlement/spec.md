## MODIFIED Requirements

### Requirement: Pending 状态记录
在执行 `buy` 或 `import` 且当日官方净值不可用时（20 天内都不可用），必须能够存储一笔 `pending` 状态的交易。

#### Scenario: 自动查找后无 NAV
- **WHEN** 用户执行 `fund buy 000300 --money 1000 --date 2026-03-13`，且 20 天内都无 NAV
- **THEN** 系统创建 pending 交易，记录原始日期
