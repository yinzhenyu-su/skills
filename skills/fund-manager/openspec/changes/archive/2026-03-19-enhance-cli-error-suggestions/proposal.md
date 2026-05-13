## Why

当前 CLI 在处理“未预期的参数”（Unexpected Argument）时，直接输出 clap 默认的英文错误信息（例如 `error: unexpected argument 'use' found`）。这与项目目前致力于提供全中文、智能建议且具备“用法示例”的友好报错目标不一致。当用户因命令顺序错误或拼写错误而触发此错误时，系统应当能智能识别意图并给出纠正建议。

## What Changes

- **拦截解析错误**：将 `main.rs` 中的 `Cli::parse()` 替换为 `Cli::try_parse()`，以便在解析阶段拦截并格式化错误信息。
- **智能建议增强**：针对 `UnknownArgument` 错误，检查该参数是否是当前命令层次下的其他有效子命令或常用参数，并提供“Did you mean?”风格的提示。
- **统一中文报错格式**：将原本英文的解析错误转换为项目统一的格式：`❌ 问题描述` → `💡 提示` → `   用法示例`。
- **错误上下文注入**：在报错信息中保留并增强当前执行命令的上下文，确保用户明确知道是在哪个子命令下出错。

## Capabilities

### New Capabilities
- `unexpected-arg-suggestions`: 提供针对未知参数的智能意图识别与建议功能。

### Modified Capabilities
- `cli-error-handling-and-advice`: 扩展错误处理标准，将解析层级的未知参数错误纳入“智能建议”规范中。

## Impact

- `src/main.rs`: 核心入口改动，涉及错误拦截与分发逻辑。
- `src/resolver.rs`: `AdviceEngine` 可能需要扩展，以支持子命令名称的模糊匹配与建议。
- `tests/cli_tests.rs`: 更新现有的错误断言测试，验证新的友好报错输出。
