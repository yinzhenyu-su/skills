## Why

在之前的 `unify-cli-error-messages` 变更中，我们统一了大部分命令的参数缺失错误提示。然而，`fund add` 命令依然使用着旧的、带有 clap 原生英文报错的参数验证方式，并且要求用户同时提供 `CODE` 和 `NAME` 这两个硬性必填参数。
这导致了两个问题：
1. **体验不统一**：`fund add` 的错误提示格式与其余命令脱节。
2. **操作繁琐**：手动提供基金名称不仅麻烦，而且容易输错。既然我们已经有了强大的 Morningstar 和 EastMoney 搜索 API，完全可以只通过基金代码或关键字进行自动检索和补全。

## What Changes

1. **废除 `<NAME>` 必填参数**：使 `fund add` 命令更智能，只需输入基金代码（或关键字）。
2. **将 `<CODE>` 参数变更为 `<FUND>` 并且设为 `Option`**：通过统一的 `require_fund_or_exit` 逻辑拦截错误，输出友好的中文提示。
3. **增加智能补全流程**：当用户仅提供一个标识符时，调用现有的 `resolve_fund`（通过 `MorningstarSearchProvider` 等）来获取正确的基金名称和代码，并自动写入本地数据库。
4. **统一错误输出格式**：缺失参数时，直接给出类似 `❌ 缺少参数：请提供基金代码` 以及用法的中文提示，替代现有的 clap 报错。

## Capabilities

### New Capabilities

### Modified Capabilities

- `smart-search`: `fund add` 将接入已有的智能搜索体系，不仅能按代码添加，还能按名称搜索添加。
- `cli-error-handling-and-advice`: 扩展错误处理体系，覆盖 `fund add` 的参数缺失场景。

## Impact

- `src/cli.rs` 中 `FundCommands::Add` 的参数签名会变化。
- `src/main.rs` 中 `FundCommands::Add` 的 handler 会增加 `resolve_fund` 的调用步骤。
- 自动化测试 `tests/cli_tests.rs` 和 `tests/cli_wallet_tests.rs` 中有关 `fund add` 的用例（可能带有固定的 name 参数）需要进行兼容性更新。
