## Why

新用户在开始使用 fund-manager 时，或老用户在定期对账时，需要将外部平台的基金持仓批量录入。手动逐个执行 `buy` 命令效率低下。提供一个批量导入（Import）命令，并支持不同的冲突解决策略（合并或覆盖），可以极大提高管理效率。

## What Changes

- **新增 `fund import` 命令**: 支持通过命令行参数对或 CSV 文件批量导入基金持仓金额。
- **冲突策略支持**: 
    - `--merge`: 将导入金额追加到现有持仓中（默认行为）。
    - `--override`: 清除现有历史记录，以导入金额作为新基准。
- **批量导入校验与非交互模式**: 自动校验 CSV 格式、金额合法性以及基金名称的唯一性匹配。为保障批量导入体验，默认采用非交互模式，遇到多匹配（歧义）时直接视为失败并记录。
- **错误报告机制**: 在导入结束后，汇总并展示导入成功与失败的详细列表及原因。

## Capabilities

### New Capabilities
- `bulk-import`: 支持从 CSV 文件或命令行参数批量解析和导入基金数据。
- `import-strategies`: 支持合并（Merge）和覆盖（Override）两种数据对齐策略。

### Modified Capabilities
- `transaction-tracking`: 增加 `import` 作为一种特殊的交易类型。
- `smart-search`: （或者 `resolver` 逻辑）引入非交互模式（non-interactive mode）用于批量处理场景。

## Impact

- `src/cli.rs`: 增加 `import` 子命令。
- `src/db.rs`: 查询逻辑中兼容 `import` 类型的交易（等同于买入逻辑，但有特定的重置行为）。
- `src/resolver.rs`: 增加 `interactive` 标志以支持静默解析。
- 新增依赖: `csv` crate 用于稳健的 CSV 解析。
