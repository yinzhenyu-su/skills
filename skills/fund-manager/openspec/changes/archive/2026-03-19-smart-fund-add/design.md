## Context

当前在 `fund-manager` 中，`fund add` 命令用于在本地数据库的 `fund` 表中插入一条新的追踪记录。目前的实现极其“裸”，只做简单的 SQL 插入，要求用户必须手动提供完整、准确的 `code` 和 `name`：
```
fund-manager fund add <CODE> <NAME>
```

随着应用其他部分（如 `preview buy`, `sell` 等）引入了智能的 `resolver`（支持根据关键字通过 Morningstar 进行模糊搜索），`fund add` 的这种硬编码体验显得落后且不一致。

## Goals / Non-Goals

**Goals:**
- 提供智能的添加体验：用户只需输入 `fund add 000300` 或者 `fund add 德邦德利`。
- 保证错误体验与全局一致：使用刚重构好的 `require_fund_or_exit` 拦截参数缺失。
- 利用现有的基础设施（`src/resolver.rs`）复用代码，不再重复造轮子。

**Non-Goals:**
- 不改变数据库 `fund` 表的设计，依然是存储 `code` 和 `name`。
- 此次不触及 `fund sync` 命令的统一（虽然它们可能也有类似问题，但超出本次 scope，可另起 change）。

## Decisions

### 1. `FundCommands::Add` 参数重构

在 `src/cli.rs` 中，现有的参数设计：
```rust
    Add {
        /// 基金代码
        code: String,
        /// 基金名称
        name: String,
        /// 初始申购费率
        #[arg(long, default_value = "0.00")]
        fee: String,
    }
```
将变更为：
```rust
    Add {
        /// 基金代码或名称（省略将给出错误提示）
        fund: Option<String>,
        /// 初始申购费率
        #[arg(long)]
        fee: Option<String>,
    }
```
*为什么要废弃 `name` 并改用单个 `fund` 标识符？*
因为 `resolver::resolve_fund` 本身就支持模糊匹配。用户无论是传入纯数字（视为 code），还是传入拼音/汉字（视为 name），resolver 都能通过 Morningstar API 找到对应的标准 `code` 和 `name` 返回。

### 2. 交互与调用逻辑

在 `src/main.rs` 的 `FundCommands::Add` 处理器中：
1. **参数拦截**：使用 `require_fund_or_exit(&conn, fund, wallet_id, "fund add")`（由于 `fund add` 是全局的，不需要 wallet_id 的过滤，这里只需传递一个占位的标识即可，可能需要微调 `require_fund_or_exit` 让它在没有可用钱包时不崩溃，或者忽略 wallet 的限制）。
2. **智能解析**：调用 `resolver::resolve_fund(&conn, &fund_input, false).await`（注意 local_only 为 `false`，因为用户要添加它，显然本地还不存在，必须允许远程 fetch）。
3. **入库操作**：拿到 `fund_obj` 后，提取它的 `code` 和 `name`，连同 `fee`（若提供）一起调用 `db::add_fund` 插入。

## Risks / Trade-offs

- **Risk**: 现有的测试极度依赖 `fund add <code> <name>` 的格式。
  **Mitigation**: 需要遍历 `tests/cli_tests.rs` 和 `tests/cli_wallet_tests.rs`，将所有的 `fund add 000300 沪深300` 替换为 `fund add 000300`。
- **Trade-off**: `fund add` 以前是完全离线的操作。现在，如果你尝试添加一个不在本地的新基金，它会产生一次网络请求调用 API。
  **Mitigation**: 这是符合用户预期的，因为要实现“通过名称搜索”，或者“获取正确的官方名称”，必然需要网络。如果不希望产生网络，用户就必须提供所有信息（这与当前应用“智能代理”的定位相悖）。
