## Context

SKILL.md 是 AI skill 的触发式文档，供 Claude Code 在对话中识别用户意图并生成对应命令调用。当前文档存在命令层级错误，AI 直接复现示例会失败。

## Goals / Non-Goals

**Goals:**
- 修正命令示例与 `cli.rs` 定义一致
- 补全缺失的常用命令 `fund config`
- 保留探索口子，不过度列举参数

**Non-Goals:**
- 不改 CLI 代码本身
- 不补全所有参数（给 AI 留探索空间）

## Decisions

1. **用实际 CLI 测试结果对齐文档** — 基于 `cli.rs` 代码而非推测
2. **market 命令补常用选项** — `--fx`, `--com`, `--detail`，让 AI 知道有这个能力但不穷举
3. **fund config 只写核心用法** — `fund fund config <基金> --dividend-mode reinvest`

## Risks / Trade-offs

- 文档仍可能随 CLI 演进而滞后 → 建议定期与 `cli.rs` 对齐
