## 1. 搬迁准备

- [x] 1.1 将 `skills/fund-manager/src/db_tests.rs` 复制到 `skills/fund-manager/tests/db_tests.rs`。

## 2. 源码清理

- [x] 2.1 从 `skills/fund-manager/src/lib.rs` 中删除 `pub mod db_tests;`。
- [x] 2.2 删除原始文件 `skills/fund-manager/src/db_tests.rs`。

## 3. 测试代码重构

- [x] 3.1 修改 `skills/fund-manager/tests/db_tests.rs`，将 `use crate::db;` 替换为 `use fund_manager::db;`。

## 4. 验证与回归

- [x] 4.1 运行 `cargo test --test db_tests` 验证数据库集成测试通过。
- [x] 4.2 运行 `cargo test` 确保所有测试依然正常运行。
