## Why

`preview` 和 `preview-sell` 是顶层命令，但文档示例却写成 `fund preview buy 000312`，导致用户运行 `fund fund preview buy` 时命令无法识别。同时 `preview-sell` 的命名与 `preview`（即买入预览）不对称，体验割裂。

## What Changes

- 将顶层 `preview` 命令重构为带子命令的嵌套结构：`preview buy` 和 `preview sell`
- 删除顶层 `preview-sell` 命令，合并到 `preview sell` 子命令下
- 更新 `cli.rs` 中的命令定义：新增 `PreviewCommands` 枚举，`fund` 参数改为 `Option<String>`
- 更新 `main.rs` 中的命令分发逻辑，新增 `resolve_fund_interactively()` 辅助函数
- 修正 `long_about` 示例文档，使其与实际调用方式一致
- 缺少 `<FUND>` 参数时：打印当前已追踪基金列表并给出用法示例，而非抛出 clap 原始错误
- 缺少 `--money`/`--shares` 参数时：提供分类说明和具体示例（sell 额外支持 `all` / 分数语法）
- 调整 `smart_nav_lookup` 回退策略：优先向后查询（最多30天），再向前查询（最多20天），适配 QDII 基金 T+1/T+2 净值延迟场景

## Capabilities

### New Capabilities

- `preview-subcommand`: `preview` 作为父命令，`buy` 和 `sell` 作为子命令的嵌套 CLI 结构

### Modified Capabilities

（无现有 spec 需要修改）

## Impact

- `src/cli.rs`：新增 `PreviewCommands` 枚举，修改 `Commands::Preview` 变体
- `src/main.rs`：更新命令分发的模式匹配逻辑
- **BREAKING**：`preview-sell` 命令被移除，需改用 `preview sell`
- 用户文档/帮助文本将自动通过 clap 更新
