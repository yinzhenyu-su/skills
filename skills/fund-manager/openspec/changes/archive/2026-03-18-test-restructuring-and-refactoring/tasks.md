## 1. 基础设施搭建 (Test Infrastructure)

- [x] 1.1 创建目录 `skills/fund-manager/tests/common/` 并初始化 `mod.rs`。
- [x] 1.2 在 `common/context.rs` 中实现 `TestContext` 结构，包含自动创建临时目录和 RAII 清理逻辑。
- [x] 1.3 为 `TestContext` 添加 `cmd()` 方法，预配置 `FUND_MANAGER_APP_DIR` 环境变量。
- [x] 1.4 在 `Cargo.toml` 中确保 `dev-dependencies` 包含必要的测试库（如 `tempfile` 如果需要增强隔离）。

## 2. 数据库测试内存化 (In-Memory DB)

- [x] 2.1 修改 `skills/fund-manager/src/db.rs` 中的 `init_db`，使其支持 `":memory:"` 路径。
- [x] 2.2 在 `db.rs` 中添加一个辅助函数 `get_test_db()`，返回一个已初始化所有表的内存数据库连接。
- [x] 2.3 将现有的 `src/db_tests.rs` 中的测试用例迁移到使用内存数据库。

## 3. Mock 数据解耦 (Fixtures)

- [x] 3.1 创建 `skills/fund-manager/tests/fixtures/` 目录。
- [x] 3.2 将各 Provider (Morningstar, Eastmoney) 中硬编码的 HTML/JSON 提取到 fixtures 文件中。
- [x] 3.3 重构 Provider 模块的 `#[cfg(test)]` 部分，使用 `include_str!` 加载这些 fixture 文件进行测试。

## 4. CLI 测试大规模精简 (CLI Refactoring)

- [x] 4.1 在 `tests/cli_tests.rs` 中引入 `common::TestContext`。
- [x] 4.2 重构 `test_wallet_add`, `test_wallet_use` 等基础命令测试，消除冗余的 setup 代码。
- [x] 4.3 将 `cli_tests.rs` 按功能模块拆分（例如：`cli_wallet_tests.rs`, `cli_fund_tests.rs`, `cli_sync_tests.rs`）。
- [x] 4.4 确保所有 CLI 测试都能在并发运行（`cargo test`）时保持路径隔离。

## 5. 业务逻辑集成测试 (Integration Tests)

- [x] 5.1 创建 `tests/integration_tests.rs`，专门测试复杂的业务流程（如买入 -> 净值同步 -> 卖出 -> 收益率计算）。
- [x] 5.2 在集成测试中优先使用内存数据库连接，以提高运行效率。

## 6. 验证与清理

- [x] 6.1 运行 `cargo test` 确保所有测试通过且无回归。
- [x] 6.2 检查测试耗时，验证内存数据库带来的性能提升。
- [x] 6.3 检查 `target/tests/` 目录，验证 `TestContext` 是否正确清理了临时文件夹。
