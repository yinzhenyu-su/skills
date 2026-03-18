## ADDED Requirements

### Requirement: 独立数据库测试执行
系统必须支持在不依赖项目源码中 `db_tests` 模块的情况下，通过外部集成测试（`tests/`）运行数据库相关的验证逻辑。

#### Scenario: 成功运行集成测试
- **WHEN** 运行 `cargo test --test db_tests`
- **THEN** 系统编译并执行 `tests/db_tests.rs` 中的所有测试用例，并报告通过。

### Requirement: 集成测试对 Public API 的访问
集成测试文件 `tests/db_tests.rs` 必须能够通过引用库 `fund_manager` 访问其公开导出的数据库操作函数。

#### Scenario: 引用 Public API 验证
- **WHEN** 测试代码中使用 `use fund_manager::db;` 访问 `db::setup_test_db()`
- **THEN** 代码必须成功编译并正确连接内存数据库。
