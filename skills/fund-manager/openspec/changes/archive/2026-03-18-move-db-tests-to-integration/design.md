## Context

目前 `src/db_tests.rs` 是通过 `src/lib.rs` 中的 `pub mod db_tests;` 声明的。这导致它在 `cargo build` 期间实际上被视为库代码的一部分，虽然它主要用于测试。这种结构在库变得复杂时不利于维护。

## Goals / Non-Goals

**Goals:**
- 将 `db_tests.rs` 重新定义为外部集成测试。
- 确保所有被测数据库函数均已通过 `pub` 暴露。
- 保持现有的测试用例逻辑不变。

**Non-Goals:**
- 不涉及数据库表结构的修改。
- 不增加新的测试用例（除非为了兼容集成测试环境）。

## Decisions

### 1. 移动文件到 `tests/`
- **决策**: 将 `src/db_tests.rs` 移动到 `tests/db_tests.rs`。
- **理由**: Rust 官方标准推荐将黑盒集成测试放在 `tests/` 目录下。

### 2. 修改导入方式
- **决策**: 使用 `use fund_manager::db;` 代替 `use crate::db;`。
- **理由**: 集成测试文件会被编译为独立的二进制文件，它们通过外部 crate 的方式访问被测项目。

### 3. 可见性维持
- **决策**: 保持 `db.rs` 中被测函数的 `pub` 可见性。
- **理由**: 经过调研，目前的函数如 `insert_nav_history_idempotent` 和 `search_funds_locally` 已经是 `pub` 的，无需修改。

## Risks / Trade-offs

- **[Risk]** → 可能存在某些测试依赖于 `db.rs` 中的私有辅助函数。
- **[Mitigation]** → 如果发现此类依赖，将对应的私有测试逻辑保留在 `src/db.rs` 的内联 `mod tests` 中。
- **[Risk]** → 编译速度可能略微受影响（集成测试是独立二进制）。
- **[Mitigation]** → 对于本项目规模，这种影响微乎其微。
