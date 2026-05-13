# 删除 `import` 命令，保留 `import-holding`

## Summary

删除 `fund-manager import` 命令（历史交易导入），保留 `import-holding` 命令（持仓快照导入）。

## Motivation

当前 fund-manager 有两个导入命令：
- `import` — 从 CSV 或命令行导入历史买入交易
- `import-holding` — 从 CSV 导入持仓快照（市值+盈亏）

两个命令业务语义不同，但功能有重叠。用户实际更常使用的是持仓导入场景。

## Scope

**删除：**
- `src/cli.rs` 中的 `Import` 枚举变体
- `src/main.rs` 中的 `Commands::Import` 分支、`handle_import` 函数、`ImportItem`/`ImportResult` 结构体
- 相关单元测试
- SKILL.md 中的 `import` 命令文档

**保留：**
- `import-holding` 命令

## Alternatives Considered

1. **合并两个命令** — 通过 `--mode` 或自动检测 CSV 格式区分，但增加了复杂度和边界情况
2. **维持现状** — 两个命令独立存在

## Status

- [x] Proposal created
- [x] Design reviewed
- [x] Implementation complete
- [x] Documentation updated

## Notes

- 原有 `import` 命令的测试已转换为 `import-holding` 测试
- `import-holding` 的参数是 `--override-flag`（不是 `--override`）
