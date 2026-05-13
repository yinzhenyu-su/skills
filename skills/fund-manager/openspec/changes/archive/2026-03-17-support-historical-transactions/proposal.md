## Why

目前 `fund-manager` 的交易记录仅限“今日”，这限制了用户补录历史交易的能力，并导致集成测试极其脆弱（依赖于运行测试时的真实系统日期）。通过支持指定日期，我们可以实现历史对账，并使测试环境完全确定。

## What Changes

- **扩展 `buy` 和 `sell` 命令**: 增加可选的 `--date` 参数，支持 `YYYY-MM-DD` 格式。
- **扩展 `import` 命令**: 
    - CSV 导入支持每行独立的日期列（可选）。
    - 命令行变长参数对支持补录日期（待设计，初步方案可能是在 CSV 模式先行）。
- **优化结算逻辑**: 根据交易指定的日期（而不是系统今日日期）来决定是立即结算还是存为 `pending`。
- **增强测试稳定性**: 所有的集成测试将使用固定的历史日期，不再受系统当前日期波动的影响。

## Capabilities

### New Capabilities
- `historical-transaction-tracking`: 支持并校验 `YYYY-MM-DD` 格式的交易日期输入。

### Modified Capabilities
- `transaction-tracking`: 允许在插入交易记录时显式指定日期，而非默认使用“今日”。
- `bulk-import`: CSV 解析器增加对可选日期列的支持。

## Impact

- `src/cli.rs`: 增加 `--date` 参数定义。
- `src/main.rs`: 修改命令分发和 `handle_import` 逻辑，处理输入日期。
- `src/db.rs`: 确保查询和插入逻辑正确处理指定的日期字符串。
- `tests/cli_tests.rs`: 重构所有受时间影响的测试用例。
