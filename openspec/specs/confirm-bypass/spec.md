## ADDED Requirements

### Requirement: Global Confirmation Skip Flag

系统 MUST 提供一个全局命令行参数 `-y` 或 `--yes` 用于跳过所有交互式确认。

#### Scenario: Using -y flag

- **WHEN** 用户执行 `fund fund delete 000300 -y`
- **THEN** 系统不再提示用户输入 "y/N" 确认。
- **AND** 系统直接执行删除操作并告知用户已通过参数确认。
