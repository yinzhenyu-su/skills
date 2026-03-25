# Design: 删除 `import` 命令

## Overview

本变更删除 `import` 命令，保留 `import-holding`。这是一个简单的删除操作，不需要架构变更。

## Deletion Scope

### 1. CLI 定义 (`src/cli.rs`)

删除 `Import` 枚举变体（lines ~105-133），保留 `ImportHolding`。

### 2. 命令分发 (`src/main.rs`)

- 删除 `Commands::Import` 分支（~lines 2085-2116）
- 删除 `handle_import` 函数（~lines 368-631）
- 删除 `ImportItem` 结构体（~lines 17-23）
- 删除 `ImportResult` 结构体（~lines 25-29）
- 删除 main.rs 中的相关单元测试（`test_import_item_structure`, `test_import_result_structure`）

### 3. 文档更新

- 更新 `SKILL.md` 移除 `import` 命令及其示例

## Data Integrity

删除命令不影响现有数据。`import-holding` 使用的 `transaction_log.type='import'` 与 `import` 的 `type='buy'` 是不同值，无冲突。

## Backward Compatibility

- 已有的 `import` 命令历史记录不受影响（数据库中交易类型为 `buy`）
- 用户迁移：`import-holding` 提供更直观的持仓导入方式

## Verification

1. `cargo build` 编译通过 ✓
2. `cargo test` 110 tests passed ✓
3. 原有 `import` 测试已转换为 `import-holding` 测试
4. `fund-manager --help` 不再显示 `import` 命令
