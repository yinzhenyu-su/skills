## Context

当前项目的测试主要集中在 `tests/cli_tests.rs` (E2E) 和 `src/*.rs` (Unit/Integration)。

**痛点细节**:
1.  **环境重复**: 每个 CLI 测试都需要手动创建 `temp_dir` 并设置环境变量。
2.  **清理困难**: 如果测试失败，临时目录往往留在磁盘上，占用空间且干扰后续测试。
3.  **数据库文件依赖**: 测试强制要求磁盘 I/O，无法在只读环境或纯内存环境中运行，也增加了测试用例间的耦合风险。
4.  **Mock 数据膨胀**: `morningstar.rs` 等模块中嵌入了大量的 HTML/JSON 长字符串，影响代码可读性。

## Goals / Non-Goals

**Goals:**
- 实现 `TestContext` 模式，将每个测试的 setup/teardown 逻辑缩减至 1 行。
- 将 CLI 测试代码量减少 40% 以上，并按功能模块拆分。
- 在 `src/db.rs` 中引入对内存数据库的支持，允许在内存中快速运行 SQL 相关测试。
- 将所有解析器的测试用例（Fixtures）统一管理到 `tests/fixtures/`。

**Non-Goals:**
- 不改变现有的业务逻辑或 SQL Schema。
- 不引入重型的测试框架（保持使用 Rust 标准的 `#[test]` 和 `assert_cmd`）。
- 不重构 `Trending Skill` (skills/trending.md)。

## Decisions

### 1. 采用 `TestContext` (RAII) 模式管理环境
- **决策**: 创建 `tests/common/context.rs`，利用 Rust 的 `Drop` trait 自动清理临时文件。
- **理由**: RAII 是 Rust 管理资源的最佳实践。通过 `ctx.cmd()` 包装 `assert_cmd::Command`，可以确保 `FUND_MANAGER_APP_DIR` 始终被正确注入。

### 2. 数据库连接的可选注入
- **决策**: 修改 `src/db.rs` 中的 `init_db` 函数，或者增加一个返回 `Result<Connection>` 的辅助函数。
- **理由**: 目前 `init_db` 逻辑包含创建表和执行迁移。通过允许指定 `:memory:` 路径，我们可以复用这些逻辑来快速创建测试环境。

### 3. 测试文件重命名与分类
- **决策**:
    - `tests/cli_tests.rs` -> 拆分为 `tests/cli_wallet_tests.rs`, `tests/cli_fund_tests.rs` 等。
    - 引入 `tests/integration_tests.rs` 专门用于调用代码 API（非 CLI 进程）的业务流测试。
- **理由**: 单个超长文件难以维护且编译速度慢。

### 4. Fixture 驱动的 Provider 测试
- **决策**: 在 `tests/fixtures/` 下存储响应样本。在测试中使用 `include_str!` 或 `std::fs::read_to_string` 加载。
- **理由**: 提高代码整洁度，方便增加更多的测试样本（如各种错误响应、空响应）。

## Risks / Trade-offs

- **[Risk]** → 测试并行运行时的冲突。
- **[Mitigation]** → 为每个 `TestContext` 生成带随机前缀或时间戳的目录名。
- **[Risk]** → 内存数据库测试与真实文件数据库的行为差异（如某些 PRAGMA 设置）。
- **[Mitigation]** → 在 `integration_tests` 中保留少量磁盘文件测试作为冒烟测试，主要测试仍使用内存模式。
- **[Trade-off]** → 引入 `tests/common/` 模块需要调整 `Cargo.toml` 或者在集成测试中显式声明 `mod common;`。
