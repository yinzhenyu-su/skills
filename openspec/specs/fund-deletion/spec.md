## MODIFIED Requirements

### Requirement: Interactive Confirmation
删除操作必须经过用户确认以防止误操作，除非显式指定了跳过标志。

#### Scenario: User cancels deletion
- **WHEN** 用户执行 `fund delete 000300` 但在提示时输入 `n`
- **THEN** 系统不执行任何删除动作。

#### Scenario: User bypasses deletion confirmation
- **WHEN** 用户执行 `fund delete 000300 -y`
- **THEN** 系统直接删除该基金及其所有关联数据，不再进行交互式提示。
