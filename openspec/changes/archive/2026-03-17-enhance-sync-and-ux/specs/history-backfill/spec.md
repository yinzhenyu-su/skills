## ADDED Requirements

### Requirement: Historical Date Range Sync
系统必须允许用户指定起始和截止日期来同步基金的历史净值。

#### Scenario: Manual range sync
- **WHEN** 用户执行 `fund sync 000300 --start 2024-01-01 --end 2024-01-10`
- **THEN** 系统抓取该代码在指定日期区间内的所有可用净值并存入 `nav_history` 表

### Requirement: Help Information with Examples
CLI 的帮助信息必须包含常见命令的使用示例。

#### Scenario: Viewing command help
- **WHEN** 用户执行 `fund buy --help`
- **THEN** 输出中必须包含 "EXAMPLES" 章节，展示如何进行自动结算、手动买入等操作
