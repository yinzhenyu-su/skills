## ADDED Requirements

### Requirement: Complete Fund Data Removal
系统必须支持通过基金代码彻底删除其所有关联数据。

#### Scenario: Deleting a fund
- **WHEN** 用户执行 `fund delete 000300` 并确认
- **THEN** 系统从 `fund` 表中移除该记录，并级联清除 `transaction_log` 和 `nav_history` 表中该基金的所有记录。

### Requirement: Interactive Confirmation
删除操作必须经过用户确认以防止误操作。

#### Scenario: User cancels deletion
- **WHEN** 用户执行 `fund delete 000300` 但在提示时输入 `n`
- **THEN** 系统不执行任何删除动作。
