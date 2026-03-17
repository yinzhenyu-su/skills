## ADDED Requirements

### Requirement: Transaction Date Input and Validation
系统必须支持通过 `--date` 参数接收交易日期，且该日期必须符合 `YYYY-MM-DD` 格式。

#### Scenario: Valid date input
- **WHEN** 用户执行 `fund buy 000300 --money 1000 --date 2024-01-01`
- **THEN** 系统解析日期成功，并将其作为该笔交易的发生日期

#### Scenario: Invalid date input
- **WHEN** 用户输入 `--date 2024-02-30` (非法日期) 或 `--date 2024/01/01` (错误格式)
- **THEN** 系统报错退出，并提示日期格式应为 YYYY-MM-DD
