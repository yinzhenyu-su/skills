# 项目概览 (Project Overview)

本项目名为 `skills`，是一个 AI 技能和工具的集合。它采用 **OpenSpec** (规范驱动开发) 流程进行管理。

核心组件包括：

1. **Fund Manager (基金管理器)**: 位于 `skills/fund-manager/`，是一个基于 Rust 开发的 CLI 工具，用于追踪和分析个人基金投资。
2. **Trending Skill (热搜技能)**: 位于 `skills/trending.md`，用于获取各主流中文平台的热搜榜单。
3. **OpenSpec 工作流**: 通过 `.gemini/commands/opsx/` 和 `.github/prompts/` 提供的指令，管理从提案到实现的完整开发生命周期。

## 技术栈 (Tech Stack)

### Fund Manager

- **语言**: Rust (2024 Edition)
- **运行时**: tokio (Async/Await)
- **数据库**: SQLite (rusqlite)
- **网络/解析**: reqwest, scraper (HTML 抓取)
- **CLI 框架**: clap
- **UI/展示**: comfy-table

### 开发工具

- **工作流管理**: OpenSpec (opsx)
- **AI 辅助**: Gemini CLI / Claude

## 构建与运行 (Building and Running)

### Fund Manager

- **编译**: `cargo build` (在 `skills/fund-manager/` 目录下运行)
- **运行**: `cargo run -- [args]`
- **测试**: `cargo test`

### OpenSpec 工作流

- **创建提案**: 使用 `opsx-propose` 命令。
- **进行设计**: 使用 `opsx-explore` 命令。
- **应用变更**: 使用 `opsx-apply` 命令。
- **归档变更**: 使用 `opsx-archive` 命令。

## 开发规范 (Development Conventions)

1. **规范驱动**: 所有重大功能变更必须先通过 `openspec/changes/` 下的 `proposal.md`、`design.md` 和 `tasks.md` 进行定义。
2. **代码风格**: 遵循 Rust 标准惯例，使用 `cargo fmt` 进行格式化。
3. **测试驱动**: 在 `skills/fund-manager/tests/` 编写集成测试，并确保 `src/` 中的 `db_tests.rs` 等单元测试通过。
4. **提供者模式**: 基金数据抓取应在 `src/provider/` 下实现对应的 trait。

## 目录结构说明 (Directory Structure)

- `openspec/`: OpenSpec 规范和变更记录。
- `skills/`:
  - `fund-manager/`: 基金管理器源代码。
  - `trending.md`: 热搜技能定义。
- `.gemini/`: Gemini CLI 专用命令和技能配置。
- `.github/prompts/`: 用于辅助开发的 GitHub 提示词模板。
