## Why

当前 CLI 在用户输入错误（如参数顺序颠倒、拼写错误）或操作受限（如余额不足）时，返回的错误信息过于生硬，缺乏引导性。这导致用户（尤其是新手）难以快速纠错，降低了工具的易用性和“人情味”。

## What Changes

- **智能参数纠错**：在 `import`、`buy`、`sell` 等命令中，当解析基金失败但后续金额参数看起来像基金代码时，主动提示参数可能反转。
- **操作可行性预检**：在 `sell` 操作前校验持有量，并提供具体的余额不足提示。
- **拼写建议 (Did you mean?)**：在 `status`、`history` 等命令中，针对未命中的基金名称提供本地相似项建议。
- **全量同步引导**：在 `sync` 命令未带参数时，提示如何使用 `--all` 进行全量同步。
- **上下文感知提示**：在报错信息中包含当前激活的钱包名称，帮助定位环境问题。

## Capabilities

### New Capabilities
- `cli-error-handling-and-advice`: 定义一套通用的 CLI 错误处理与建议生成逻辑，包括参数反转检测和拼写建议。

### Modified Capabilities
- `transaction-tracking`: 增加交易前的余额预校验要求。
- `metadata-sync`: 细化 `sync` 命令在无参数情况下的交互行为。

## Impact

- `skills/fund-manager/src/resolver.rs`: 增强 `resolve_fund` 的错误返回，支持携带诊断信息。
- `skills/fund-manager/src/main.rs`: 在各个命令的处理函数中接入建议引擎。
- `skills/fund-manager/src/cli.rs`: 可能需要微调部分命令的 `long_about` 以包含更多引导。
- `skills/fund-manager/src/db.rs`: 增加支持拼写建议的模糊搜索接口。
