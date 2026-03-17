## MODIFIED Requirements

### Requirement: CSV File Import
系统必须支持通过 `--file` 参数接收并解析 CSV 文件，文件应包含基金名称/代码和金额两列，并支持可选的第三列作为交易日期。

#### Scenario: Valid CSV format with dates
- **WHEN** 用户执行 `fund import --file data.csv`，且文件内容为 `000300,5000,2024-01-01`
- **THEN** 系统将该笔 5000 元的导入记录在 2024-01-01

#### Scenario: CSV without dates (backward compatibility)
- **WHEN** 用户提供的 CSV 只有两列 `000300,5000`
- **THEN** 系统使用命令行全局指定的 `--date` 或默认使用“今日”作为交易日期
