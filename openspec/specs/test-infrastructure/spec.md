## ADDED Requirements

### Requirement: TestContext 隔离管理
系统必须提供一个 `TestContext` 结构，用于在测试开始时自动创建唯一的临时工作目录，并在测试结束时自动清理该目录（除非显式配置保留以供调试）。

#### Scenario: 自动创建与清理
- **WHEN** 实例化一个 `TestContext::new("test-name")`
- **THEN** 系统在 `target/tests/` 下创建一个名为 `test-name_<uuid>` 的唯一目录，并在该 Context 实例被销毁（Drop）时自动递归删除该目录。

### Requirement: 预配置的 Command 包装器
`TestContext` 必须能够提供预先配置好环境变量（特别是 `FUND_MANAGER_APP_DIR`）的 `assert_cmd::Command` 实例。

#### Scenario: Command 环境注入
- **WHEN** 调用 `ctx.cmd()` 获取命令实例
- **THEN** 该命令实例的环境变量 `FUND_MANAGER_APP_DIR` 必须指向该 Context 的临时目录，确保测试操作与主机环境完全隔离。
