## MODIFIED Requirements

### Requirement: Sync Without Arguments Guide
当用户执行 `fund fund sync` 命令但未提供具体的基金标识符时，系统 SHALL 引导用户使用全量同步参数，而不是仅仅报错。

#### Scenario: Sync guidance without arguments
- **WHEN** 用户执行 `fund fund sync`（无参数）
- **THEN** 系统 SHALL 提示：`💡 提示：运行 'fund fund sync --all' 可以同步所有持有基金的元数据。`
