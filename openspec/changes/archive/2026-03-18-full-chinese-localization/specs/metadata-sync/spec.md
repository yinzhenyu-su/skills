## ADDED Requirements

### Requirement: 汉化同步进度日志
系统在进行批量或单体基金元数据同步时，SHALL 使用中文显示同步进度和操作状态。

#### Scenario: 同步中的实时日志
- **WHEN** 正在同步代码为 "000300" 的基金
- **THEN** 系统 SHALL 在控制台显示：正在同步 000300 (沪深300) ...
