## Context

目前 `fund-manager` 使用 `clap` 默认的 `Cli::parse()`。当用户输入错误（如拼写错误或顺序不对）时，`clap` 会直接输出默认的英文错误信息并退出。这不符合项目“友好中文提示”的规范。

## Goals / Non-Goals

**Goals:**
- 拦截并本地化 `clap` 的解析错误。
- 针对 `UnknownArgument` 提供“Did you mean?”风格的建议。
- 识别子命令错位的情况并给出修正建议（例如 `wallet list use` -> `wallet use`）。
- 保持与现有 `AdviceEngine` 设计理念一致。

**Non-Goals:**
- 不打算完全重写 `clap` 的所有错误类型，仅针对最常见的 `UnknownArgument`、`MissingRequiredArgument` 等进行优化。
- 不会改变 `clap` 的底层解析逻辑。

## Decisions

### 1. 使用 `try_parse()` 替代 `parse()`
在 `main.rs` 入口处调用 `Cli::try_parse()`，通过 `match` 处理 `Err` 分支。这样可以获取到完整的 `clap::Error` 对象，包括错误类型和上下文。

### 2. 智能建议逻辑扩展
在 `AdviceEngine` 中新增处理解析错误的方法。逻辑如下：
- **子命令匹配**：如果错误是 `UnknownArgument`，且该参数在父级子命令的合法列表中，提示用户是否写错了位置。
- **模糊匹配**：使用 Levenshtein 距离（或简单的包含关系）对比输入与有效指令集。

### 3. 统一输出格式
所有拦截后的错误，按如下模板输出：
```
❌ [中文错误描述]
💡 Hint: [建议信息] (可选)
   用法示例：[Correct Example]
```

## Risks / Trade-offs

- **[Risk]**：手动处理 `clap` 错误可能会导致在 `clap` 版本升级时出现 API 不兼容。
  - **Mitigation**：仅依赖 `clap::ErrorKind` 和 `error.get_context()`，这些是相对稳定的 API。
- **[Trade-off]**：为了提供建议，需要动态获取 `clap` 命令结构。
  - **Decision**：利用 `Cli::command()` 导出的 `Command` 对象，它包含了完整的元数据，无需硬编码指令列表。
