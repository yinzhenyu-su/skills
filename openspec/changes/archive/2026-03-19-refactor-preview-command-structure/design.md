## Context

当前 `fund-manager` 有两个顶层预览命令：

- `preview`（买入预览，`Commands::PreviewBuy`）
- `preview-sell`（卖出预览，`Commands::PreviewSell`）

问题在于文档示例写的是 `fund preview buy 000312`，但实际 clap 结构并不支持这种调用方式——`buy` 会被当作基金名称而非子命令。`preview-sell` 这个连字符命名也让命令不直观。

与此同时，`buy` 和 `sell` 是顶层命令，所以 `preview buy` / `preview sell` 在语义上最自然——它们是对应操作的"干跑（dry-run）"版本。

## Goals / Non-Goals

**Goals:**

- `preview buy <fund>` 和 `preview sell <fund>` 作为统一入口
- 删除 `preview-sell` 顶层命令
- 帮助文本示例与实际调用方式一致

**Non-Goals:**

- 调整 `buy` / `sell` 命令本身的行为
- 将 `preview` 移到 `fund fund` 子命令树下

## Decisions

### 决策 1：新增 `PreviewCommands` 枚举

在 `cli.rs` 中新增：

```rust
#[derive(Subcommand)]
pub enum PreviewCommands {
    /// 预览买入结果（不执行实际买入）
    Buy { fund, money, shares, nav, date, wallet }
    /// 预览卖出结果（不执行实际卖出）
    Sell { fund, money, shares, nav, date, wallet }
}
```

`Commands::Preview` 变体持有 `PreviewCommands` 作为子命令，而非直接持有参数。

**为什么不用两个独立顶层命令？**  
两个独立命令（`preview` 和 `preview-sell`）在自动补全和帮助文本中不够对称，且和文档意图不符。嵌套方式让 `preview --help` 可以直接展示 `buy` 和 `sell` 两个子命令。

### 决策 2：`main.rs` 分发调整

将原来的：

```rust
Commands::PreviewBuy { .. } => handle_preview_buy(...)
Commands::PreviewSell { .. } => handle_preview_sell(...)
```

改为：

```rust
Commands::Preview { command } => match command {
    PreviewCommands::Buy { .. } => handle_preview_buy(...)
    PreviewCommands::Sell { .. } => handle_preview_sell(...)
}
```

## Risks / Trade-offs

### 决策 3：`fund` 参数改为 `Option<String>` + `resolve_fund_interactively()`

将 `PreviewCommands::Buy/Sell` 中的 `fund` 字段由 `String`（必填）改为 `Option<String>`（可选），并在命令分发层调用 `resolve_fund_interactively(conn, fund, wallet_id, subcommand)` 处理缺失情况。

缺少 `fund` 时的行为：

- 从数据库查询当前钱包已追踪的基金列表
- 若列表非空：打印每条 `基金名 (代码)` 到 stderr，并附带用法示例
- 若列表为空：提示用户先用 `fund fund add` 添加基金
- 最终调用 `std::process::exit(1)` 退出

**为什么不用 clap required 字段？**  
clap 默认的 `missing argument` 错误缺乏上下文（不告诉用户有哪些基金可选），改为 `Option` 后可以提供更友好的引导信息，同时成本低（只需一个辅助函数）。

### 决策 4：`smart_nav_lookup` 采用向后优先的回退策略

当指定日期的净值在本地 DB 和远端 API 均不可用时，回退顺序调整为：

1. 向后查最近 30 天（`find_prev_available_nav`）—— 覆盖 QDII 基金 T+1/T+2 净值延迟
2. 若仍未找到，向前查最多 20 天（`find_next_available_nav`）—— 兜底处理

**为什么向后优先？**  
用户预览买入/卖出时，通常期望使用最近已公布的净值（而非未来日期的净值）。QDII 基金当天净值往往次日才入库，向后查询能命中 T-1 数据，符合直觉。

### 决策 5：缺少 `--money`/`--shares` 时提供分类错误提示

`handle_preview_buy` 和 `handle_preview_sell` 在两个参数均缺失时，替换原有单行错误为多行说明：

- **buy**：展示 `--money <金额>` 和 `--shares <份额>` 两个选项及示例
- **sell**：额外展示 `--shares all`（全仓）和 `--shares 1/2`（分数份额）语法

这属于对内部逻辑的小范围修改，目的是与 `resolve_fund_interactively` 的风格保持一致——所有缺参场景都给出可操作的提示。

- **[Breaking change] `preview-sell` 命令被移除** → 现有用的用户需改用 `preview sell`；影响范围小（本地 CLI 工具，无外部 API）
- **[测试影响] cli_tests.rs 中如有对 `preview-sell` 的测试需更新** → 作为 tasks 中的步骤包含
