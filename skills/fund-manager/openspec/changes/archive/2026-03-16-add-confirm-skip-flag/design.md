## Context

当前系统中的确认逻辑分散在不同的命令处理器中，且通常依赖硬编码的 `stdin` 读取。随着功能的增加，重复编写确认逻辑不仅冗余，也使得添加全局跳过功能变得困难。

## Goals / Non-Goals

**Goals:**
- 在顶层 CLI (`src/cli.rs`) 引入全局 `-y` / `--yes` 参数。
- 在 `main.rs` 或独立的工具函数中统一处理“确认并继续”的判断逻辑。
- 确保所有子命令均能感知并尊重该全局参数。

**Non-Goals:**
- 不涉及改变现有交互式确认的交互文案（除非为了结构化）。
- 不改变默认行为（默认依然需要确认）。

## Decisions

### 1. 全局参数注入 (Global Flag)
- **Decision**: 使用 `clap` 的 `#[arg(global = true)]` 属性。
- **Rationale**: 这样可以避免在每个子命令中重复定义该参数，且允许用户在命令行的任何位置放置 `-y`。

### 2. 确认逻辑抽象
- **Decision**: 在 `main.rs` 中或作为一个宏/函数实现 `confirm_action(prompt: &str, force_yes: bool)`。
- **Logic**:
    - 如果 `force_yes` 为真 -> 打印提示 -> 返回真。
    - 否则 -> 打印提示 -> 读取 `stdin` -> 返回用户选择的结果。

## Risks / Trade-offs

- **[Risk] 意外破坏性操作** → **Mitigation**: `-y` 参数是用户显式输入的，且在跳过确认时仍会打印一条简短的告知信息，确保用户知情。
