## MODIFIED Requirements

### Requirement: Historical Date Range Sync
系统必须允许用户指定起始和截止日期来同步基金的历史净值。同步过程中的反馈日志 SHALL 统一为中文。

#### Scenario: Syncing history with Chinese logs
- **WHEN** 执行历史净值同步
- **THEN** 系统 SHALL 输出：`正在同步 000300 (基金名称) ...` 以及 `✓ 已同步 N 天的历史净值`。
