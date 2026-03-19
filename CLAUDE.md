# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概览

skills 是一个 AI 技能和工具的 monorepo，采用 **OpenSpec**（规范驱动开发）流程管理功能变更。

核心组件：

- `skills/fund-manager/` — Rust CLI 基金管理工具（详见其 [CLAUDE.md](skills/fund-manager/CLAUDE.md)）
- `skills/trending.md` — 热搜技能定义（微博、知乎、头条、抖音、百度）
- `openspec/` — OpenSpec 规范和变更记录

## 常用命令

### Fund Manager（在 `skills/fund-manager/` 下运行）

```bash
cargo build            # 构建
cargo run -- [args]    # 运行
cargo test             # 测试
```

### OpenSpec 工作流

通过 Claude Code skills 或 `.claude/commands/opsx/` 中的命令使用：

- `opsx-propose` — 创建变更提案（proposal.md + design.md + tasks.md）
- `opsx-explore` — 探索模式，思考和调查问题
- `opsx-apply` — 实施变更任务
- `opsx-archive` — 完成后归档变更

变更生命周期：propose → explore → apply → archive

归档变更位于 `openspec/changes/archive/`，活跃变更位于 `openspec/changes/`。

## 目录结构

```
.
├── openspec/
│   ├── config.yaml           # OpenSpec 配置（schema: spec-driven）
│   ├── specs/                # 活跃规范（各含 spec.md）
│   └── changes/
│       ├── archive/          # 已归档变更（各含 proposal.md, design.md, tasks.md）
│       └── <active-changes>/ # 进行中的变更
├── skills/
│   ├── fund-manager/         # Rust CLI 基金管理工具
│   └── trending.md           # 热搜技能定义
├── .claude/                  # Claude Code 配置
│   ├── commands/opsx/        # opsx 命令定义
│   └── skills/               # 技能定义
├── .gemini/                  # Gemini CLI 配置
└── .github/prompts/          # GitHub Copilot 提示词
```

## 开发规范

1. **规范驱动** — 重大功能变更先通过 OpenSpec 流程定义，再实施
2. **Rust 代码风格** — 使用 `cargo fmt` 格式化，遵循标准惯例
3. **提交信息** — 使用 feat/fix/docs 等标准类型，归档变更用 `docs(openspec): archive change <name>`
4. **归档文件需提交** — `openspec/changes/archive/` 下的文件也要提交到仓库
