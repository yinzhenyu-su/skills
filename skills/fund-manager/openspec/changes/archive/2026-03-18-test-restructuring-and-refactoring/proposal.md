## Why

当前 `fund-manager` 的测试体系存在严重的技术债：测试用例散落在各个源文件中，`cli_tests.rs` 极其庞大（超过 1000 行）且包含大量重复的环境初始化代码。数据库测试依赖于真实的磁盘文件 I/O，导致测试运行缓慢且难以完全隔离。

通过重构，我们将建立一个结构化、高性能且易于维护的测试框架，为后续复杂功能的开发提供质量保证。

## What Changes

- **测试基础设施**: 引入 `TestContext` 辅助工具，自动管理临时测试目录和环境配置。
- **测试分层**: 将测试划分为单元测试（纯逻辑）、集成测试（业务流程）和 CLI 测试（命令行交互）。
- **数据库测试优化**: 支持使用 SQLite `:memory:` 进行内存化测试，提升性能并确保绝对隔离。
- **Mock 数据解耦**: 将解析器的测试数据（HTML/JSON）从代码中移至 `tests/fixtures/` 目录。
- **CLI 测试重构**: 使用 `TestContext` 大幅精简 `cli_tests.rs`，并按功能拆分文件。

## Capabilities

### New Capabilities
- `test-infrastructure`: 提供统一的测试上下文管理、临时环境自动清理和 Command 包装器。
- `data-fixtures-management`: 提供统一的外部数据 Mock 文件加载机制。
- `in-memory-database-testing`: 提供在内存中运行数据库迁移和业务逻辑测试的能力。

### Modified Capabilities
- 无：本项目不涉及业务逻辑需求的变更，仅为测试实现层面的重构。

## Impact

- **`tests/` 目录结构**: 将新增 `common/`, `fixtures/` 目录，并拆分 `cli_tests.rs`。
- **`src/db.rs`**: 需要微调以支持更灵活的数据库连接注入。
- **构建系统**: 测试运行速度将显著提升，测试覆盖率将更加清晰。
- **开发效率**: 新功能的测试编写将变得更加简单快捷。
