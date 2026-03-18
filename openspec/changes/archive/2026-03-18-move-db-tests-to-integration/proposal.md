## Why

当前 `db_tests.rs` 位于 `src/` 目录下并作为库的一部分被包含，这模糊了单元测试与集成测试的界限。通过将其移动到 `tests/` 目录，我们可以确保数据库逻辑通过 Public API 是可验证的，并遵循 Rust 项目的最佳实践。

## What Changes

- **代码移动**: 将 `src/db_tests.rs` 移动到 `tests/db_tests.rs`。
- **模块解构**: 从 `src/lib.rs` 中删除 `pub mod db_tests;`。
- **导入重构**: 将测试代码中的 `use crate::db;` 修改为 `use fund_manager::db;`。

## Capabilities

### New Capabilities
- `db-integration-tests`: 提供独立的数据库集成测试，确保数据库 Public API 的正确性。

### Modified Capabilities
- 无

## Impact

- **项目结构**: `src/` 目录将完全由业务逻辑组成。
- **测试运行**: 数据库测试将作为独立的集成测试二进制文件运行。
- **可见性**: 数据库 Public API 必须保持暴露，以便集成测试能够调用。
