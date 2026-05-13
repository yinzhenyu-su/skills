## Context

`fund reset` 命令当前实现为 `FundCommands::Reset`，用户需要使用 `fund-manager fund reset` 调用。但用户期望的路径是 `fund-manager reset`（顶层命令）。

此外，`cli.rs` 中多处 `long_about` 示例使用了错误的前缀 `fund status`、`fund buy` 等。正确的顶层命令应使用 `fund-manager` 前缀，只有 `fund` 子命令内部的调用（如 `fund fund add`）才保持 `fund` 前缀。

## Goals / Non-Goals

**Goals:**
- 将 `reset` 从 `FundCommands` 移至 `Commands` 顶层
- 修正所有 `long_about` 示例中的命令前缀
- 改进 `reset` 命令的帮助信息

**Non-Goals:**
- 不改变任何命令的实际功能逻辑
- 不修改 `fund` 子命令下的示例（如 `fund fund add`）
- 不修改已经在使用 `fund-manager` 前缀的示例

## Decisions

### 1. reset 命令路径

**选择：`Commands::Reset` 顶层命令**

| 方案 | 路径 | 问题 |
|------|------|------|
| 保持 `FundCommands` | `fund-manager fund reset` | 不符合用户心理模型 |
| 移至顶层 | `fund-manager reset` | 更简洁，与 `status`、`history` 等平级 |

### 2. 示例前缀规范

**规则：**
- 顶层命令示例：`fund-manager <command>`
- wallet 子命令示例：`fund-manager wallet <subcommand>`
- fund 子命令示例：`fund-manager fund <subcommand>`

**需要修正的示例：**

| 原文本 | 正确文本 |
|--------|----------|
| `fund status` | `fund-manager status` |
| `fund history` | `fund-manager history` |
| `fund buy` | `fund-manager buy` |
| `fund sell` | `fund-manager sell` |
| `fund import-holding` | `fund-manager import-holding` |
| `fund dividend` | `fund-manager dividend` |
| `fund reinvest` | `fund-manager reinvest` |
| `fund market` | `fund-manager market` |
| `fund wallet add` | `fund-manager wallet add` |
| `fund reset` | `fund-manager reset` |

**保持不变的示例（fund 子命令内部调用）：**
- `fund fund add`
- `fund fund delete`
- `fund fund sync`
- `fund fund inspect`
- `fund fund config`

### 3. reset 帮助信息改进

**改进内容：**
- 列举具体删除项：钱包、基金、交易历史、净值记录、配置
- 强调不可恢复
- 示例使用完整 `fund-manager reset` 路径

## Risks / Trade-offs

- **风险**：现有用户可能习惯 `fund reset` 路径 → **无兼容方案**，reset 是危险操作，需要明确路径
- **风险**：OpenSpec 归档文档引用需要同步更新 → 已在 Impact 中列出

## Open Questions

无
