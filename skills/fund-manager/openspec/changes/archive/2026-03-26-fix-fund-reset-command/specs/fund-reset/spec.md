## MODIFIED Requirements

### Requirement: Fund reset command

The system SHALL provide a `fund-manager reset` command that permanently deletes all personal data from the local database, preserving the database schema structure for future use.

#### Scenario: Reset with confirmation
- **WHEN** user executes `fund-manager reset` without `-y` flag
- **THEN** system prompts for confirmation: "确定要重置所有数据吗？这将永久删除所有钱包、基金、交易历史、净值记录和配置，且无法恢复！"
- **AND** waits for user input before proceeding

#### Scenario: Reset with force flag
- **WHEN** user executes `fund-manager reset -y`
- **THEN** system skips confirmation prompt and immediately proceeds with reset
- **AND** prints "正在重置所有数据..."

#### Scenario: Reset deletes all personal data
- **WHEN** user confirms reset (or uses `-y`)
- **THEN** system closes the database connection
- **AND** deletes the database file
- **AND** reinitializes an empty database with schema
- **AND** prints "✅ 数据已重置，所有个人数据已被永久删除。"

#### Scenario: Reset completes successfully
- **WHEN** reset operation finishes
- **THEN** system exits with code 0
- **AND** user can start fresh with empty database
