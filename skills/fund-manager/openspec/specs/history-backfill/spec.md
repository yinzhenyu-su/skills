## ADDED Requirements

### Requirement: Historical Date Range Sync
系统必须允许用户指定起始和截止日期来同步基金的历史净值。同步过程中的反馈日志 SHALL 统一为中文。

#### Scenario: Syncing history with Chinese logs
- **WHEN** 执行历史净值同步
- **THEN** 系统 SHALL 输出：`正在同步 000300 (基金名称) ...` 以及 `✓ 已同步 N 天的历史净值`。

### Requirement: Help Information with Examples
CLI 的帮助信息必须包含常见命令的使用示例。

#### Scenario: Viewing command help
- **WHEN** 用户执行 `fund buy --help`
- **THEN** 输出中必须包含 "EXAMPLES" 章节，展示如何进行自动结算、手动买入等操作
